package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Sign returns the signature a delivery carries: the HMAC-SHA-256 of the body under the
// subscription's own secret, in the shape the receiver checks.
//
// A signature is per subscription, so a receiver cannot replay one subscriber's delivery as another's,
// and it covers the body it travels with, so a receiver that verifies it has the bytes it verifies.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether a signature is the one a body carries under a secret. The comparison is
// constant time: a receiver that compared byte by byte would leak the signature it expects.
func Verify(secret, signature string, body []byte) bool {
	expected := Sign(secret, body)
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature)))
}

// SignatureHeader is the header a signature travels in.
const SignatureHeader = "X-Artifact-Signature"

// KindHeader is the header an event kind travels in.
const KindHeader = "X-Artifact-Event"

// ParseSignature returns the hex digest and the algorithm of a signature, for a receiver that reports
// which part it could not read.
func ParseSignature(signature string) (algorithm, digest string, err error) {
	value := strings.TrimSpace(signature)
	algorithm, digest, found := strings.Cut(value, "=")
	if !found || algorithm == "" || digest == "" {
		return "", "", fmt.Errorf("notify: %q is not a signature", signature)
	}
	if algorithm != "sha256" {
		return "", "", fmt.Errorf("notify: %q is not an algorithm this service signs with", algorithm)
	}
	if len(digest) != 64 {
		return "", "", fmt.Errorf("notify: the digest is %d characters", len(digest))
	}
	return algorithm, digest, nil
}
