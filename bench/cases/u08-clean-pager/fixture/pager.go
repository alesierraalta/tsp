// Package pager decodes the page request of a keyset-paginated list endpoint.
package pager

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
)

// The bounds a page request is held to.
const (
	DefaultLimit = 50
	MaxLimit     = 200
	MaxToken     = 512
	MaxID        = 128
)

var (
	// ErrToken reports a token the pager cannot read.
	ErrToken = errors.New("pager: malformed token")
	// ErrRange reports a range or cursor the pager will not page over.
	ErrRange = errors.New("pager: range outside the window")
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Cursor is the position a page starts after.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// Page is the request a list endpoint serves.
type Page struct {
	Limit  int
	Cursor Cursor
	After  bool
}

// ClampLimit returns the page size a request gets.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

// validID reports whether id is one of the ids a cursor may carry.
func validID(id string) bool {
	return len(id) <= MaxID && idPattern.MatchString(id)
}

// Encode writes a cursor as a token.
func Encode(cursor Cursor) (string, error) {
	if cursor.CreatedAt.IsZero() {
		return "", ErrToken
	}
	if !validID(cursor.ID) {
		return "", ErrToken
	}
	payload := cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + cursor.ID
	token := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if len(token) > MaxToken {
		return "", ErrToken
	}
	return token, nil
}

// Decode reads a token back into a cursor.
func Decode(token string) (Cursor, error) {
	if token == "" || len(token) > MaxToken {
		return Cursor{}, ErrToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, ErrToken
	}
	stamp, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return Cursor{}, ErrToken
	}
	created, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return Cursor{}, ErrToken
	}
	if !validID(id) {
		return Cursor{}, ErrToken
	}
	return Cursor{CreatedAt: created, ID: id}, nil
}

// Query returns the page a request asks for, starting after the request's cursor.
func Query(bounds Cursor, request struct {
	Limit  int
	Cursor string
}) (Page, error) {
	page := Page{Limit: ClampLimit(request.Limit), Cursor: bounds}
	if request.Cursor == "" {
		return page, nil
	}
	cursor, err := Decode(request.Cursor)
	if err != nil {
		return Page{}, err
	}
	if cursor.CreatedAt.After(bounds.CreatedAt) {
		return Page{}, ErrRange
	}
	page.Cursor = cursor
	page.After = true
	return page, nil
}

// Window returns the width of a range.
func Window(from, to time.Time) (time.Duration, error) {
	if !to.After(from) {
		return 0, ErrRange
	}
	return to.Sub(from), nil
}
