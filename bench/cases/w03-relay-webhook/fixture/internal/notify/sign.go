package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// signatureVersion is the prefix of the signature header the endpoints verify.
const signatureVersion = "v1"

// SignatureHeader is the header the signature travels in.
const SignatureHeader = "X-Relay-Signature"

// Sign renders the signature of one body under one secret. The signing key is the subscription's
// own secret, so one endpoint's signature is worthless to another.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return signatureVersion + "=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature against a body.
func Verify(secret string, body []byte, signature string) bool {
	version, hexPart, ok := strings.Cut(signature, "=")
	if !ok || version != signatureVersion {
		return false
	}
	expected := Sign(secret, body)
	return hmac.Equal([]byte(expected), []byte(signature)) && hexPart != ""
}

// Analyse explains a signature a receiver rejected, without printing the secret.
func Analyse(secret string, body []byte, signature string) string {
	if Verify(secret, body, signature) {
		return "signature matches"
	}
	if !strings.HasPrefix(signature, signatureVersion+"=") {
		return "signature has no version prefix"
	}
	if len(signature) <= len(signatureVersion)+1 {
		return "signature is empty"
	}
	return "signature does not match the body"
}

// ParseTimestamp reads the timestamp parameter some endpoints send back with a signature.
func ParseTimestamp(raw string) (time.Time, error) {
	seconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("notify: timestamp %q", raw)
	}
	return time.Unix(seconds, 0).UTC(), nil
}
