// Package idgen mints the identifiers the archive stores: document ids, batch ids and trail
// entries. They are random, prefixed and fixed length, so a caller cannot tell one kind from
// another by guessing and an id sort is a creation order.
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Prefixes the service uses. A prefix is part of the id, not a separate column.
const (
	Doc   = "doc"
	Batch = "bat"
	Entry = "ent"
	Page  = "pg"
	Trail = "aud"
	Token = "tok"
)

// length is the number of random bytes behind an id. Twelve bytes is short enough for a URL and
// long enough that two scans of the same tree never collide.
const length = 12

// New mints an id with prefix and length random bytes. It panics only when the system entropy
// source fails, which no deployment can continue past.
func New(prefix string) string {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		panic("idgen: read random: " + err.Error())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

// Valid reports whether id carries the prefix and a full-length hex body. A handler reads an id
// from a URL, so the shape is checked before the id is used as a map key or a filename component.
func Valid(prefix, id string) bool {
	head, body, ok := strings.Cut(id, "-")
	if !ok || head != prefix {
		return false
	}
	if len(body) != length*2 {
		return false
	}
	if _, err := hex.DecodeString(body); err != nil {
		return false
	}
	return true
}

// MustNew mints an id and panics when the entropy source fails, for the call sites that have no
// error path of their own (a batch id in a constructor).
func MustNew(prefix string) string { return New(prefix) }

// PageName renders the sidecar name a page index uses: page-007.pgm for the eighth page.
func PageName(page int) string {
	return fmt.Sprintf("page-%03d.pgm", page)
}

// SeqName renders the zero-padded sequence number a staging directory writes for one entry.
func SeqName(seq int) string { return fmt.Sprintf("%04d", seq) }
