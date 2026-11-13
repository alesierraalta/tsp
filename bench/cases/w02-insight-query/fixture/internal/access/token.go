package access

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

// Errors a token parser reports.
var (
	ErrMalformedToken = errors.New("access: malformed token")
	ErrBadSignature   = errors.New("access: bad signature")
	ErrExpiredToken   = errors.New("access: token expired")
)

// Token is a decoded and verified bearer token.
type Token struct {
	Subject string
	Tenant  string
	Scopes  []string
	Expires time.Time
}

// Issuer signs and verifies the tokens the gateway hands out.
type Issuer struct {
	secret []byte
	now    func() time.Time
}

// NewIssuer returns an issuer over a shared secret. A secret shorter than 16 bytes is refused: a
// short key makes the signature guessable, and a guessable signature makes the subject a choice of
// the caller.
func NewIssuer(secret string) (*Issuer, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("%w: secret shorter than 16 bytes", ErrMalformedToken)
	}
	return &Issuer{secret: []byte(secret), now: func() time.Time { return time.Now().UTC() }}, nil
}

// SetClock replaces the clock the issuer reads.
func (i *Issuer) SetClock(now func() time.Time) {
	if now != nil {
		i.now = now
	}
}

// Issue signs a token for a subject, a tenant and a scope list.
func (i *Issuer) Issue(subject, tenant string, scopes []string, ttl time.Duration) string {
	payload := strings.Join([]string{
		"v1", subject, tenant, strings.Join(scopes, " "), strconv.FormatInt(i.now().Add(ttl).Unix(), 10),
	}, "|")
	return payload + "." + i.sign(payload)
}

func (i *Issuer) sign(payload string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify parses and verifies a token. The signature is compared before any field is trusted.
func (i *Issuer) Verify(raw string) (Token, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Token{}, fmt.Errorf("%w: empty", ErrMalformedToken)
	}
	cut := strings.LastIndex(raw, ".")
	if cut <= 0 || cut == len(raw)-1 {
		return Token{}, fmt.Errorf("%w: no signature", ErrMalformedToken)
	}
	payload, sig := raw[:cut], raw[cut+1:]
	if !hmac.Equal([]byte(sig), []byte(i.sign(payload))) {
		return Token{}, fmt.Errorf("%w", ErrBadSignature)
	}
	fields := strings.Split(payload, "|")
	if len(fields) != 5 || fields[0] != "v1" {
		return Token{}, fmt.Errorf("%w: %d fields", ErrMalformedToken, len(fields))
	}
	expires, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return Token{}, fmt.Errorf("%w: expiry %q", ErrMalformedToken, fields[4])
	}
	tok := Token{
		Subject: fields[1],
		Tenant:  fields[2],
		Scopes:  strings.Fields(fields[3]),
		Expires: time.Unix(expires, 0).UTC(),
	}
	if tok.Subject == "" || tok.Tenant == "" {
		return Token{}, fmt.Errorf("%w: empty subject or tenant", ErrMalformedToken)
	}
	if !i.now().Before(tok.Expires) {
		return Token{}, fmt.Errorf("%w: expired at %s", ErrExpiredToken, tok.Expires.Format(time.RFC3339))
	}
	return tok, nil
}

// Bearer extracts the token from an Authorization header value.
func Bearer(header string) (string, error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(rest) == "" {
		return "", fmt.Errorf("%w: header %q", ErrMalformedToken, header)
	}
	return strings.TrimSpace(rest), nil
}
