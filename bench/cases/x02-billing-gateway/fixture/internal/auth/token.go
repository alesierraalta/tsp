// Package auth issues and verifies the tokens the billing gateway reads and holds the policy that says
// which scope a route needs. A token names one subject, one tenant and a set of scopes; it is signed
// with the deployment's secret and nothing in it is read before the signature is checked.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/billing/internal/clock"
)

// Errors a token reports.
var (
	// ErrToken is returned for a token that is malformed.
	ErrToken = errors.New("auth: malformed token")
	// ErrSignature is returned for a token whose signature does not verify.
	ErrSignature = errors.New("auth: the token signature does not verify")
	// ErrExpired is returned for a token whose expiration is not in the future.
	ErrExpired = errors.New("auth: the token has expired")
	// ErrSecret is returned when a deployment has no signing secret.
	ErrSecret = errors.New("auth: the signing secret is empty")
)

// Claims are what a token asserts.
type Claims struct {
	Subject string   `json:"sub"`
	Tenant  string   `json:"ten"`
	Scopes  []string `json:"scp"`
	Issued  int64    `json:"iat"`
	Expires int64    `json:"exp"`
}

// Validate checks the claims a token carries once its signature has been verified.
func (c Claims) Validate() error {
	if strings.TrimSpace(c.Subject) == "" {
		return fmt.Errorf("%w: no subject", ErrToken)
	}
	if strings.TrimSpace(c.Tenant) == "" {
		return fmt.Errorf("%w: no tenant", ErrToken)
	}
	if c.Expires <= c.Issued {
		return fmt.Errorf("%w: it expires before it was issued", ErrToken)
	}
	for _, scope := range c.Scopes {
		if !ValidScope(scope) {
			return fmt.Errorf("%w: scope %q", ErrToken, scope)
		}
	}
	return nil
}

// Issuer signs and verifies tokens for one deployment.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    clock.Clock
}

// NewIssuer returns an issuer over a secret and a default lifetime. The secret is copied: a caller
// that replaces its own slice does not change the key the deployment signs with.
func NewIssuer(secret string, ttl time.Duration, now clock.Clock) (*Issuer, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("%w: %d bytes", ErrSecret, len(secret))
	}
	if now == nil {
		return nil, fmt.Errorf("auth: an issuer needs a clock")
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, now: now}, nil
}

// SetClock replaces the clock the issuer reads expiration from.
func (i *Issuer) SetClock(now clock.Clock) {
	if now != nil {
		i.now = now
	}
}

// TTL returns the default lifetime the issuer gives a token.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue signs a token for a subject in a tenant with the scopes given. A lifetime of zero means the
// issuer's default; a lifetime beyond the default is refused, because a caller that could ask for a
// token that never expires would outlive the credential rotation it is meant to survive.
func (i *Issuer) Issue(subject, tenant string, scopes []string, ttl time.Duration) (string, error) {
	if i == nil || len(i.secret) == 0 {
		return "", ErrSecret
	}
	if ttl <= 0 {
		ttl = i.ttl
	}
	if ttl > i.ttl {
		return "", fmt.Errorf("auth: a lifetime of %s exceeds the %s maximum", ttl, i.ttl)
	}
	set, err := ParseScopes(scopes)
	if err != nil {
		return "", err
	}
	issued := i.now().UTC()
	claims := Claims{
		Subject: subject,
		Tenant:  tenant,
		Scopes:  set.Names(),
		Issued:  issued.Unix(),
		Expires: issued.Add(ttl).Unix(),
	}
	if err := claims.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + i.sign(encoded), nil
}

// Verify checks a token's signature and lifetime and returns its claims. The signature is checked
// before the claims are read: a token that fails it is not decoded at all.
func (i *Issuer) Verify(token string) (Claims, error) {
	encoded, signature, found := strings.Cut(strings.TrimSpace(token), ".")
	if !found || encoded == "" || signature == "" {
		return Claims{}, ErrToken
	}
	expected := i.sign(encoded)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return Claims{}, ErrSignature
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrToken, err)
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrToken, err)
	}
	if err := claims.Validate(); err != nil {
		return Claims{}, err
	}
	if !i.now().UTC().Before(time.Unix(claims.Expires, 0).UTC()) {
		return Claims{}, fmt.Errorf("%w: it expired at %s", ErrExpired, clock.Format(time.Unix(claims.Expires, 0)))
	}
	return claims, nil
}

// sign returns the signature of an encoded payload, base64url encoded without padding.
func (i *Issuer) sign(encoded string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(encoded))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
