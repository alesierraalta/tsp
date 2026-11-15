// Package api is the HTTP surface: the routes, the token check every request passes, and the error
// mapping that keeps an internal message out of a response.
package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"example.com/ledgerclose/internal/auth"
	"example.com/ledgerclose/internal/calendar"
	"example.com/ledgerclose/internal/config"
	"example.com/ledgerclose/internal/httpx"
	"example.com/ledgerclose/internal/identity"
	"example.com/ledgerclose/internal/invoice"
	"example.com/ledgerclose/internal/journal"
	"example.com/ledgerclose/internal/notify"
	"example.com/ledgerclose/internal/pdfgen"
	"example.com/ledgerclose/internal/pricing"
	"example.com/ledgerclose/internal/query"
	"example.com/ledgerclose/internal/store"
	"example.com/ledgerclose/internal/usage"
	"example.com/ledgerclose/internal/workspace"
)

// maxAuthorizationBytes bounds the Authorization header before it is parsed.
const maxAuthorizationBytes = 4096

// Deps are the parts the surface is wired from.
type Deps struct {
	Config    config.Config
	Directory *identity.Directory
	Book      *pricing.Book
	Store     *store.Store
	Journal   *journal.Journal
	Workspace *workspace.Workspace
	Renderer  *pdfgen.Runner
	Notify    *notify.Client
	Issuer    *auth.Issuer
}

// API is one wired surface.
type API struct {
	cfg       config.Config
	limits    usage.Limits
	directory *identity.Directory
	book      *pricing.Book
	store     *store.Store
	trail     *journal.Journal
	workspace *workspace.Workspace
	renderer  *pdfgen.Runner
	notifier  *notify.Client
	issuer    *auth.Issuer
	policy    *auth.Policy
	resolver  *identity.Resolver
	now       func() time.Time
	mux       *http.ServeMux
	routeList []string
}

// New wires the surface. Every dependency the handlers use is required: a nil one is refused here,
// so no handler has to decide what a missing store means.
func New(deps Deps) (*API, error) {
	if err := deps.Config.Validate(); err != nil {
		return nil, err
	}
	if deps.Directory == nil || deps.Book == nil || deps.Store == nil || deps.Journal == nil ||
		deps.Workspace == nil || deps.Issuer == nil {
		return nil, errors.New("api: the surface needs a directory, a book, a store, a journal, a workspace and an issuer")
	}
	// The caps are checked as a value: a zero Limits would accept a body of no bytes and serve a page
	// of no rows, which reads as a working deployment that answers nothing.
	caps := deps.Config.Limits()
	if !caps.Valid() {
		return nil, fmt.Errorf("api: the caps are not usable: %+v", caps)
	}
	a := &API{
		cfg: deps.Config, limits: caps, directory: deps.Directory, book: deps.Book, store: deps.Store,
		trail: deps.Journal, workspace: deps.Workspace, renderer: deps.Renderer, notifier: deps.Notify,
		issuer: deps.Issuer, policy: auth.NewPolicy(), resolver: identity.NewResolver(deps.Directory),
		now: func() time.Time { return time.Now().UTC() },
	}
	a.mux = a.routes()
	return a, nil
}

// Handler returns the HTTP handler, with the middleware every request passes through.
func (a *API) Handler(logger *log.Logger) http.Handler {
	var emit func(format string, args ...any)
	if logger != nil {
		emit = logger.Printf
	}
	return httpx.Requests(emit, 30*time.Second, a.mux)
}

// Routes lists what the service serves.
func (a *API) Routes() []string { return append([]string(nil), a.routeList...) }

// SetClock replaces the clock the handlers stamp and compare with.
func (a *API) SetClock(now func() time.Time) {
	if now != nil {
		a.now = now
		a.issuer.SetClock(now)
	}
}

// principal verifies the token of a request and decides whether it may perform an action. The order
// is the contract: the header is bounded, the token is verified (signature first), the subject is
// resolved against the directory byte for byte and in the tenant the token asserts, the tenant is
// one this deployment bills, and only then is the scope checked. The account the token names is
// returned with the principal so a handler does not resolve it a second time.
func (a *API) principal(w http.ResponseWriter, r *http.Request, action string) (auth.Principal, identity.Account, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		httpx.Fail(w, http.StatusUnauthorized, "a bearer token is required")
		return auth.Principal{}, identity.Account{}, false
	}
	scheme, token, ok := strings.Cut(header, " ")
	if len(header) > maxAuthorizationBytes || !ok || !strings.EqualFold(scheme, "bearer") ||
		strings.TrimSpace(token) != token {
		httpx.Fail(w, http.StatusUnauthorized, "invalid token")
		return auth.Principal{}, identity.Account{}, false
	}
	claims, err := a.issuer.Verify(token)
	if err != nil {
		message := "invalid token"
		if errors.Is(err, auth.ErrExpired) {
			message = "expired token"
		}
		httpx.Fail(w, http.StatusUnauthorized, message)
		return auth.Principal{}, identity.Account{}, false
	}
	account, err := a.resolver.ResolveIn(claims.Subject, claims.Tenant)
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "invalid token")
		return auth.Principal{}, identity.Account{}, false
	}
	if !a.cfg.TenantAllowed(claims.Tenant) {
		a.auditRefusal(claims.Subject, claims.Tenant, action)
		httpx.Fail(w, http.StatusForbidden, "the tenant is not billed by this deployment")
		return auth.Principal{}, identity.Account{}, false
	}
	principal := auth.Principal{Subject: claims.Subject, Tenant: claims.Tenant, Scopes: auth.NewScopeSet(claims.Scopes)}
	if err := a.policy.Check(principal, action); err != nil {
		a.auditRefusal(principal.Subject, principal.Tenant, action)
		httpx.Fail(w, http.StatusForbidden, "the caller does not hold the scope for this action")
		return auth.Principal{}, identity.Account{}, false
	}
	return principal, account, true
}

// auditRefusal records a refused token or scope. The trail keeps refusals as well as actions, and a
// failure to record one is not a reason to answer the request.
func (a *API) auditRefusal(actor, tenant, action string) {
	_, _ = a.trail.Append(journal.Entry{
		At: a.now(), Actor: actor, Tenant: tenant, Action: action, Ref: "auth",
	})
}

// fail maps an error to a response. The message of an unmapped error is not written to the client:
// an internal message can carry a path, a query or an account it should not.
func (a *API) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrForbidden), errors.Is(err, identity.ErrTenantMismatch):
		httpx.Fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, workspace.ErrNoObject), errors.Is(err, store.ErrUnknown):
		httpx.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, usage.ErrTooLarge), errors.Is(err, httpx.ErrBodyTooLarge),
		errors.Is(err, store.ErrTooManyRows), errors.Is(err, invoice.ErrTooMany):
		httpx.Fail(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, store.ErrDuplicate):
		httpx.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, notify.ErrResponse), errors.Is(err, notify.ErrPayload):
		httpx.Fail(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, workspace.ErrOutside), errors.Is(err, workspace.ErrBadName),
		errors.Is(err, usage.ErrInvalid), errors.Is(err, usage.ErrMalformed), errors.Is(err, usage.ErrDuplicate),
		errors.Is(err, query.ErrUnknown), errors.Is(err, query.ErrValue), errors.Is(err, query.ErrLimit),
		errors.Is(err, notify.ErrBlocked), errors.Is(err, notify.ErrMalformed),
		errors.Is(err, pricing.ErrInvalid), errors.Is(err, config.ErrInvalid),
		errors.Is(err, invoice.ErrInvalid), errors.Is(err, invoice.ErrEmpty), errors.Is(err, invoice.ErrPeriod),
		errors.Is(err, calendar.ErrMalformed):
		httpx.Fail(w, http.StatusBadRequest, err.Error())
	default:
		httpx.Fail(w, http.StatusInternalServerError, "internal error")
	}
}

// routes registers the surface, and the list is what the health endpoint publishes.
func (a *API) routes() *http.ServeMux {
	mux := http.NewServeMux()
	register := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, handler)
		a.routeList = append(a.routeList, pattern)
	}
	register("GET /healthz", a.handleHealth)
	register("POST /api/usage", a.handleIngest)
	register("GET /api/export/usage.csv", a.handleUsageCSV)
	register("POST /api/close", a.handleClose)
	register("GET /api/invoices/{id}/statement", a.handleStatement)
	register("GET /api/invoices/{id}/statement.pdf", a.handleStatementFile)
	register("POST /api/deliveries", a.handleDeliver)
	return mux
}

// usageDefinition is the one relation the reports select from. Every column, filter and ordering a
// caller may use is declared here, so the builder has a table to look a name up in.
func (a *API) usageDefinition() query.Definition {
	return query.Definition{
		Table: "usage_events",
		Columns: []string{
			store.ColumnID, store.ColumnAccount, store.ColumnMetric,
			store.ColumnQuantity, store.ColumnCurrency, store.ColumnOccurred,
		},
		Params: []query.Param{
			{Name: "account", Column: store.ColumnAccount, Kind: query.KindText},
			{Name: "metric", Column: store.ColumnMetric, Kind: query.KindEnum, Enum: usage.Metrics},
			{Name: "currency", Column: store.ColumnCurrency, Kind: query.KindEnum, Enum: identity.Currencies},
		},
		Sorts: map[string]string{
			"occurred": store.ColumnOccurred, "account": store.ColumnAccount, "quantity": store.ColumnQuantity,
		},
		MaxLimit: a.limits.PageSize, DefaultLimit: a.limits.PageSize,
	}
}
