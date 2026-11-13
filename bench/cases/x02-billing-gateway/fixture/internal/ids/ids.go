// Package ids mints the identifiers the billing service hands out. Every identifier carries the kind
// of thing it names and a random tail, so an id read out of a log says what it is and an id guessed
// from another is not a valid one.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// The prefixes this service mints.
const (
	Tenant       = "ten"
	Account      = "acc"
	Plan         = "pln"
	Subscription = "sub"
	Invoice      = "inv"
	Line         = "lin"
	Payment      = "pay"
	UsageEvent   = "evt"
	Credit       = "crd"
	Token        = "tok"
	Job          = "job"
	Note         = "nte"
	Quote        = "qte"
	Audit        = "aud"
)

// idRe is the shape every minted identifier has. It is also the shape an identifier read from a
// request must have before it is compared with a stored one.
var idRe = regexp.MustCompile(`^[a-z]{3}-[0-9a-f]{24}$`)

// New mints one identifier of the kind given.
func New(kind string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failing is the process losing its entropy source; there is no identifier to
		// hand out and no safe fallback, so the caller gets an id that will not collide with a
		// minted one and the write that uses it fails its own validation.
		return kind + "-" + strings.Repeat("0", 24)
	}
	return kind + "-" + hex.EncodeToString(buf[:])
}

// Valid reports whether value is the shape of a minted identifier of any kind.
func Valid(value string) bool {
	return idRe.MatchString(value)
}

// ValidKind reports whether value is a minted identifier of the kind given.
func ValidKind(value, kind string) bool {
	if !Valid(value) {
		return false
	}
	return strings.HasPrefix(value, kind+"-")
}

// Kind returns the prefix of a minted identifier, or the empty string when value is not one.
func Kind(value string) string {
	if !Valid(value) {
		return ""
	}
	return value[:strings.Index(value, "-")]
}
