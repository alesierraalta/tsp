// Package auth reads the bearer tokens the service accepts and decides what a caller may do.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Errors the HTTP layer maps to 401 and 403.
var (
	ErrMalformed = errors.New("auth: malformed token")
	ErrSignature = errors.New("auth: bad signature")
	ErrExpired   = errors.New("auth: token expired")
	ErrForbidden = errors.New("auth: scope not granted")
)

// Token is a decoded and verified bearer token.
type Token struct {
	Subject string
	Tenant  string
	Scopes  []string
	Expires time.Time
}

// Issuer signs and verifies tokens with one shared secret.
type Issuer struct {
	secret []byte
	now    func() time.Time
}

// NewIssuer returns an issuer over secret. A secret shorter than 16 bytes is refused: the token
// format is not a password, and a short key makes the signature guessable.
func NewIssuer(secret string) (*Issuer, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("%w: secret shorter than 16 bytes", ErrMalformed)
	}
	return &Issuer{secret: []byte(secret), now: func() time.Time { return time.Now().UTC() }}, nil
}

// SetClock replaces the clock the issuer reads. It is used by the suite to make expiry explicit.
func (i *Issuer) SetClock(now func() time.Time) {
	if now != nil {
		i.now = now
	}
}

// Issue signs a token for a subject, a tenant and a scope list.
func (i *Issuer) Issue(subject, tenant string, scopes []string, ttl time.Duration) string {
	expires := i.now().Add(ttl).Unix()
	payload := strings.Join([]string{
		"v1",
		subject,
		tenant,
		strings.Join(scopes, " "),
		strconv.FormatInt(expires, 10),
	}, "|")
	return payload + "." + i.sign(payload)
}

func (i *Issuer) sign(payload string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify parses and verifies a token. The signature is compared before any field is trusted, and
// the payload is split only after the comparison: a token whose fields the service reads before
// verifying would let a caller choose its own tenant.
func (i *Issuer) Verify(raw string) (Token, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Token{}, fmt.Errorf("%w: empty", ErrMalformed)
	}
	cut := strings.LastIndex(raw, ".")
	if cut <= 0 || cut == len(raw)-1 {
		return Token{}, fmt.Errorf("%w: no signature", ErrMalformed)
	}
	payload, sig := raw[:cut], raw[cut+1:]
	if !hmac.Equal([]byte(sig), []byte(i.sign(payload))) {
		return Token{}, fmt.Errorf("%w: %q", ErrSignature, sig)
	}
	fields := strings.Split(payload, "|")
	if len(fields) != 5 || fields[0] != "v1" {
		return Token{}, fmt.Errorf("%w: %d fields", ErrMalformed, len(fields))
	}
	expires, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return Token{}, fmt.Errorf("%w: expiry %q", ErrMalformed, fields[4])
	}
	tok := Token{
		Subject: fields[1],
		Tenant:  fields[2],
		Scopes:  strings.Fields(fields[3]),
		Expires: time.Unix(expires, 0).UTC(),
	}
	if tok.Subject == "" || tok.Tenant == "" {
		return Token{}, fmt.Errorf("%w: empty subject or tenant", ErrMalformed)
	}
	if !i.now().Before(tok.Expires) {
		return Token{}, fmt.Errorf("%w: expired at %s", ErrExpired, tok.Expires.Format(time.RFC3339))
	}
	return tok, nil
}

// Bearer extracts the token from an Authorization header value.
func Bearer(header string) (string, error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("%w: scheme %q", ErrMalformed, scheme)
	}
	if strings.TrimSpace(rest) == "" {
		return "", fmt.Errorf("%w: empty bearer", ErrMalformed)
	}
	return strings.TrimSpace(rest), nil
}
