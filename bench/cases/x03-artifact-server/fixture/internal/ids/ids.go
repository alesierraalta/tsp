// Package ids mints the identifiers the artifact server hands out.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// The prefixes this service mints.
const (
	Artifact     = "art"
	Job          = "job"
	Subscription = "sub"
	Event        = "evt"
	Audit        = "aud"
	Pack         = "pak"
)

// idRe is the shape every minted identifier has.
var idRe = regexp.MustCompile(`^[a-z]{3}-[0-9a-f]{24}$`)

// New mints one identifier of the kind given.
func New(kind string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return kind + "-" + strings.Repeat("0", 24)
	}
	return kind + "-" + hex.EncodeToString(buf[:])
}

// Valid reports whether value is the shape of a minted identifier.
func Valid(value string) bool { return idRe.MatchString(value) }

// ValidKind reports whether value is a minted identifier of the kind given.
func ValidKind(value, kind string) bool {
	return Valid(value) && strings.HasPrefix(value, kind+"-")
}

// Kind returns the prefix of a minted identifier, or the empty string.
func Kind(value string) string {
	if !Valid(value) {
		return ""
	}
	return value[:strings.Index(value, "-")]
}
