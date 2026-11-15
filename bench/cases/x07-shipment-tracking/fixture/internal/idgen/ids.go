// Package idgen mints the identifiers the service hands out. An identifier is a prefix, an
// underscore and 16 random bytes in lowercase hex, so a caller can see what a value names and the
// service can refuse a value that was not minted here.
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrEntropy is reported when the system cannot supply random bytes.
var ErrEntropy = errors.New("idgen: no entropy")

// idRE is the shape New produces, anchored on both ends.
var idRE = regexp.MustCompile(`^[a-z]{2,12}_[0-9a-f]{32}$`)

// New mints an identifier with the given prefix.
func New(prefix string) (string, error) {
	if !validPrefix(prefix) {
		return "", fmt.Errorf("idgen: prefix %q", prefix)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("%w: %v", ErrEntropy, err)
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}

// Valid reports whether raw has the shape New produces. It is the form rule; a caller that needs to
// know an identifier belongs to this store looks it up as well.
func Valid(raw string) bool { return idRE.MatchString(raw) }

// Prefix returns the part of an identifier before the underscore, lowercased only when the whole
// value is well formed. A malformed value returns "" so no caller can split one by hand.
func Prefix(raw string) string {
	if !Valid(raw) {
		return ""
	}
	prefix, _, _ := strings.Cut(raw, "_")
	return prefix
}

func validPrefix(prefix string) bool {
	if len(prefix) < 2 || len(prefix) > 12 {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
