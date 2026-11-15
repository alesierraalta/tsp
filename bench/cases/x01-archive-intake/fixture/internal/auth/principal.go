package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"example.com/archive/internal/clock"
	"fmt"
	"strings"
	"time"
)

// Principal is the verified identity of one request.
type Principal struct {
	Subject string    `json:"subject"`
	Tenant  string    `json:"tenant"`
	Scopes  []string  `json:"scopes"`
	Issued  time.Time `json:"issued"`
	Expires time.Time `json:"expires"`
}

// Has reports whether the principal carries a scope. The comparison is exact: a scope is granted by
// the string it is, so "documents:readonly" grants nothing that "documents:read" requires.
func (p Principal) Has(scope string) bool {
	for _, granted := range p.Scopes {
		if granted == scope {
			return true
		}
	}
	return false
}

// Describe renders the principal for an audit record.
func (p Principal) Describe() string {
	return fmt.Sprintf("%s@%s [%s]", p.Subject, p.Tenant, strings.Join(p.Scopes, " "))
}

// Issuer signs and verifies tokens. The secret is the deployment's; a token is one base64url payload,
// a dot, and the hex HMAC of the payload.
type Issuer struct {
	secret []byte
	now    clock.Clock
}

// NewIssuer builds an issuer. A secret shorter than sixteen bytes is refused: a token signed with a
// short secret is a token anyone can mint.
func NewIssuer(secret string) (*Issuer, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("auth: token secret is shorter than 16 bytes")
	}
	return &Issuer{secret: []byte(secret)}, nil
}

// SetClock replaces the clock the issuer reads expiration with.
func (i *Issuer) SetClock(now clock.Clock) { i.now = now }

// Issue signs a token for a subject in a tenant with the scopes given.
func (i *Issuer) Issue(subject, tenant string, scopes []string, ttl time.Duration) string {
	now := clock.Now(i.now)
	principal := Principal{
		Subject: subject,
		Tenant:  tenant,
		Scopes:  append([]string(nil), scopes...),
		Issued:  now,
		Expires: now.Add(ttl),
	}
	return i.sign(principal)
}

// sign renders and signs one principal. A marshal error cannot happen for this struct; the empty
// payload it would produce is refused by Verify.
func (i *Issuer) sign(principal Principal) string {
	raw, err := json.Marshal(payload{
		Subject: principal.Subject,
		Tenant:  principal.Tenant,
		Scopes:  principal.Scopes,
		Issued:  principal.Issued.UTC().Unix(),
		Expires: principal.Expires.UTC().Unix(),
	})
	if err != nil {
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded + "." + i.mac(encoded)
}

// payload is the wire form of a principal: instants as Unix seconds, scopes as a list.
type payload struct {
	Subject string   `json:"sub"`
	Tenant  string   `json:"ten"`
	Scopes  []string `json:"scp"`
	Issued  int64    `json:"iat"`
	Expires int64    `json:"exp"`
}

// mac renders the signature of an encoded payload.
func (i *Issuer) mac(encoded string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(encoded))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a token's shape, its signature and its expiry, in that order, and returns the principal
// it carries. Nothing in the payload is trusted before the signature matches.
func (i *Issuer) Verify(token string) (Principal, error) {
	encoded, signature, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || encoded == "" || signature == "" {
		return Principal{}, ErrMalformed
	}
	if !hmac.Equal([]byte(i.mac(encoded)), []byte(signature)) {
		return Principal{}, ErrSignature
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Principal{}, ErrMalformed
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Principal{}, ErrMalformed
	}
	if p.Subject == "" {
		return Principal{}, ErrSubject
	}
	if p.Expires <= 0 {
		return Principal{}, ErrMalformed
	}
	expires := time.Unix(p.Expires, 0).UTC()
	if !clock.Now(i.now).Before(expires) {
		return Principal{}, ErrExpired
	}
	return Principal{
		Subject: p.Subject,
		Tenant:  p.Tenant,
		Scopes:  p.Scopes,
		Issued:  time.Unix(p.Issued, 0).UTC(),
		Expires: expires,
	}, nil
}
