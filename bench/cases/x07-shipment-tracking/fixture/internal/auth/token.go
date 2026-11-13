// Package auth issues and verifies the tokens a caller presents and decides what a token may do. A
// token is `v1.<payload>.<signature>`: the payload is base64url JSON and the signature is an
// HMAC-SHA256 over the version and the payload. Verification checks the signature before it decodes
// the payload, so a caller cannot make the service parse a body of its own choosing by presenting a
// token; and the fields are then compared exactly, so a scope of `statements:readonly` grants nothing
// that `statements:read` requires.
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
)

// Errors a verification reports. The handlers map them to 401 and 403; nothing else reads them.
var (
	ErrToken   = errors.New("auth: invalid token")
	ErrExpired = errors.New("auth: token expired")
	ErrScope   = errors.New("auth: scope refused")
)

// version is the token format this build issues and accepts. A token carrying another version is
// refused rather than read by the current rules: the version is inside the signed bytes for exactly
// that reason.
const version = "v1"

// minSecret is the shortest secret an issuer accepts.
const minSecret = 16

// maxToken is the longest token the verifier reads. A longer string is refused before it is split,
// so a request cannot make the service allocate a body the size of its own header.
const maxToken = 4096

// Claims is what a verified token asserts.
type Claims struct {
	Subject string   `json:"sub"`
	Tenant  string   `json:"tenant"`
	Scopes  []string `json:"scopes"`
	Expiry  int64    `json:"exp"`
	Issued  int64    `json:"iat"`
}

// Issuer signs and verifies tokens with one secret and one lifetime.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

// NewIssuer returns an issuer. The secret is copied and must be at least 16 bytes; the lifetime must
// be positive and no longer than a year, so a token cannot outlive the deployment's key rotation.
func NewIssuer(secret []byte, ttl time.Duration) (*Issuer, error) {
	if len(secret) < minSecret {
		return nil, fmt.Errorf("%w: secret is %d bytes, need %d", ErrToken, len(secret), minSecret)
	}
	if ttl <= 0 || ttl > 365*24*time.Hour {
		return nil, fmt.Errorf("%w: lifetime %s", ErrToken, ttl)
	}
	copied := make([]byte, len(secret))
	copy(copied, secret)
	return &Issuer{secret: copied, ttl: ttl}, nil
}

// TTL is the lifetime the issuer signs into a token.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue signs a token for a subject, a tenant and a set of scopes. The scopes are carried as given:
// the issuer does not normalize a scope, because a normalized scope is a scope the policy did not
// grant.
func (i *Issuer) Issue(subject, tenant string, scopes []string) string {
	now := time.Now().UTC()
	claims := Claims{
		Subject: subject,
		Tenant:  tenant,
		Scopes:  append([]string(nil), scopes...),
		Expiry:  now.Add(i.ttl).Unix(),
		Issued:  now.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		// A Claims value of only strings and integers cannot fail to marshal; returning an empty token
		// makes the caller's verification fail closed rather than issue one with no expiry.
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signed := version + "." + encoded
	return signed + "." + i.sign(signed)
}

// IssueAt signs a token as if it were issued at a given instant, so a test can mint an expired one
// without waiting.
func (i *Issuer) IssueAt(subject, tenant string, scopes []string, at time.Time) string {
	issued := at.UTC()
	claims := Claims{
		Subject: subject,
		Tenant:  tenant,
		Scopes:  append([]string(nil), scopes...),
		Expiry:  issued.Add(i.ttl).Unix(),
		Issued:  issued.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signed := version + "." + encoded
	return signed + "." + i.sign(signed)
}

func (i *Issuer) sign(value string) string {
	mac := hmac.New(sha256.New, i.secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify checks a token and returns its claims. The order is the point: the version and the
// signature are checked first over the exact bytes the caller presented, and only then is the payload
// decoded. The expiry is compared with now, and a token that lives longer than the issuer's lifetime
// — which the issuer never creates — is refused as well, so a token signed with a leaked key cannot
// be made to last forever.
func (i *Issuer) Verify(token string, now time.Time) (Claims, error) {
	if token == "" || len(token) > maxToken {
		return Claims{}, fmt.Errorf("%w: %d bytes", ErrToken, len(token))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("%w: %d parts", ErrToken, len(parts))
	}
	if parts[0] != version {
		return Claims{}, fmt.Errorf("%w: version %q", ErrToken, parts[0])
	}
	signed := parts[0] + "." + parts[1]
	want := i.sign(signed)
	// The comparison is constant time, because the signature is a secret-derived value. hmac.Equal
	// compares the decoded bytes of both sides so a difference anywhere costs the same.
	gotBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: signature is not base64url", ErrToken)
	}
	wantBytes, err := base64.RawURLEncoding.DecodeString(want)
	if err != nil || !hmac.Equal(gotBytes, wantBytes) {
		return Claims{}, fmt.Errorf("%w: signature", ErrToken)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: payload is not base64url", ErrToken)
	}
	var claims Claims
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return Claims{}, fmt.Errorf("%w: payload %v", ErrToken, err)
	}
	if decoder.More() {
		return Claims{}, fmt.Errorf("%w: payload carries a second value", ErrToken)
	}
	if claims.Subject == "" || strings.TrimSpace(claims.Subject) != claims.Subject {
		return Claims{}, fmt.Errorf("%w: subject", ErrToken)
	}
	if strings.TrimSpace(claims.Tenant) == "" {
		return Claims{}, fmt.Errorf("%w: tenant", ErrToken)
	}
	if claims.Expiry <= 0 || claims.Issued <= 0 {
		return Claims{}, fmt.Errorf("%w: no lifetime", ErrToken)
	}
	if claims.Expiry-claims.Issued > int64(i.ttl/time.Second)+60 {
		return Claims{}, fmt.Errorf("%w: lifetime longer than the issuer's", ErrToken)
	}
	expiry := time.Unix(claims.Expiry, 0).UTC()
	if !now.UTC().Before(expiry) {
		return Claims{}, fmt.Errorf("%w: expired at %s", ErrExpired, expiry.Format(time.RFC3339))
	}
	claims.Scopes = append([]string(nil), claims.Scopes...)
	return claims, nil
}
