// Package auth issues and verifies the tokens the artifact server reads and holds the scopes a route
// requires.
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

	"example.com/artifacts/internal/clock"
)

// Errors a token reports.
var (
	// ErrToken is returned for a token that is malformed.
	ErrToken = errors.New("auth: malformed token")
	// ErrSignature is returned for a token whose signature does not verify.
	ErrSignature = errors.New("auth: the token signature does not verify")
	// ErrExpired is returned for an expired token.
	ErrExpired = errors.New("auth: the token has expired")
)

// Claims are what a token asserts.
type Claims struct {
	Subject string   `json:"sub"`
	Project string   `json:"prj"`
	Scopes  []string `json:"scp"`
	Issued  int64    `json:"iat"`
	Expires int64    `json:"exp"`
}

// Validate checks the claims after the signature has been verified.
func (c Claims) Validate() error {
	if strings.TrimSpace(c.Subject) == "" {
		return fmt.Errorf("%w: no subject", ErrToken)
	}
	if strings.TrimSpace(c.Project) == "" {
		return fmt.Errorf("%w: no project", ErrToken)
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

// NewIssuer returns an issuer over a secret and a default lifetime.
func NewIssuer(secret string, ttl time.Duration, now clock.Clock) (*Issuer, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("auth: the signing secret is %d bytes", len(secret))
	}
	if now == nil {
		return nil, fmt.Errorf("auth: an issuer needs a clock")
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, now: now}, nil
}

// SetClock replaces the issuer's clock.
func (i *Issuer) SetClock(now clock.Clock) {
	if now != nil {
		i.now = now
	}
}

// TTL returns the default lifetime.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue signs a token. A lifetime beyond the deployment's default is refused.
func (i *Issuer) Issue(subject, project string, scopes []string, ttl time.Duration) (string, error) {
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
		Subject: subject, Project: project, Scopes: set.Names(),
		Issued: issued.Unix(), Expires: issued.Add(ttl).Unix(),
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

// Verify checks a token's signature and lifetime before its claims are read.
func (i *Issuer) Verify(token string) (Claims, error) {
	encoded, signature, found := strings.Cut(strings.TrimSpace(token), ".")
	if !found || encoded == "" || signature == "" {
		return Claims{}, ErrToken
	}
	if !hmac.Equal([]byte(signature), []byte(i.sign(encoded))) {
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

// sign returns the signature of an encoded payload.
func (i *Issuer) sign(encoded string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(encoded))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
