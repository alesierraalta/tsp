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

	"example.com/ledgerclose/internal/identity"
)

// Errors a token reports. Every one is a refusal without detail: a caller that gets "invalid token"
// learns nothing about which part of it was wrong.
var (
	ErrToken      = errors.New("auth: invalid token")
	ErrExpired    = errors.New("auth: expired token")
	ErrWeakSecret = errors.New("auth: token secret is too short")
)

// tokenVersion is the only token format this build accepts.
const tokenVersion = "v1"

// maxTokenBytes and maxPayloadBytes bound the token before it is decoded and the payload the JSON
// decoder sees: a signature check over an unbounded string is work an unauthenticated caller asks
// for.
const (
	maxTokenBytes   = 4096
	maxPayloadBytes = 1024
)

// Claims are the fields a token carries.
type Claims struct {
	Subject string   `json:"sub"`
	Tenant  string   `json:"tenant"`
	Scopes  []string `json:"scopes"`
	Expiry  int64    `json:"exp"`
}

// Issuer signs and verifies tokens with one secret.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewIssuer returns an issuer. A secret shorter than 16 bytes is refused: the signature is the only
// thing that makes a token ours, and a short secret is a guessable one.
func NewIssuer(secret string, ttl time.Duration) (*Issuer, error) {
	if len(secret) < 16 {
		return nil, ErrWeakSecret
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("%w: ttl %v", ErrToken, ttl)
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, now: func() time.Time { return time.Now().UTC() }}, nil
}

// SetClock replaces the clock the issuer stamps and checks tokens with.
func (i *Issuer) SetClock(now func() time.Time) {
	if now != nil {
		i.now = now
	}
}

// Issue signs a token for a subject. The scopes are sorted before signing, so two calls with the
// same set produce the same bytes.
func (i *Issuer) Issue(subject, tenant string, scopes []string) string {
	claims := Claims{
		Subject: subject,
		Tenant:  tenant,
		Scopes:  NewScopeSet(scopes).List(),
		Expiry:  i.now().Add(i.ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString(i.sign(encoded))
	return tokenVersion + "." + encoded + "." + signature
}

// Verify returns the claims of a token this issuer signed. The order is the guard: the length is
// bounded, the parts are split, the signature is checked in constant time, and only then is the
// payload decoded — a token whose signature does not hold is refused before any field of it is
// read.
func (i *Issuer) Verify(token string) (Claims, error) {
	if token == "" || len(token) > maxTokenBytes {
		return Claims{}, fmt.Errorf("%w: length %d", ErrToken, len(token))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenVersion {
		return Claims{}, fmt.Errorf("%w: %d parts", ErrToken, len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: signature is not base64", ErrToken)
	}
	if !hmac.Equal(signature, i.sign(parts[1])) {
		return Claims{}, fmt.Errorf("%w: signature", ErrToken)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) > maxPayloadBytes {
		return Claims{}, fmt.Errorf("%w: payload", ErrToken)
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return Claims{}, fmt.Errorf("%w: payload", ErrToken)
	}
	if !identity.ValidSubject(claims.Subject) || strings.TrimSpace(claims.Tenant) == "" ||
		len(claims.Tenant) > 64 || len(claims.Scopes) == 0 || len(claims.Scopes) > 32 {
		return Claims{}, fmt.Errorf("%w: fields", ErrToken)
	}
	now := i.now()
	if claims.Expiry <= now.Unix() {
		return Claims{}, fmt.Errorf("%w: expired at %d", ErrExpired, claims.Expiry)
	}
	// A token that outlives the issuer's own lifetime was not signed by this configuration, and is
	// refused rather than honoured for its whole claim.
	if claims.Expiry > now.Add(2*i.ttl).Unix() {
		return Claims{}, fmt.Errorf("%w: expiry beyond the issuer's lifetime", ErrToken)
	}
	return claims, nil
}

// sign is the HMAC over the version and the encoded payload.
func (i *Issuer) sign(encodedPayload string) []byte {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(tokenVersion + "." + encodedPayload))
	return mac.Sum(nil)
}
