// Package pagination reads the page a listing request asks for and renders the cursor the next page
// is fetched with. A cursor is the id of the last row a page carried, encoded so a client cannot edit
// it into an id the tenant does not own: the value is compared against the tenant's rows exactly like
// any other filter value.
package pagination

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Page is one page request.
type Page struct {
	// Limit is how many rows the caller asked for.
	Limit int
	// After is the id the page starts after; empty for the first page.
	After string
}

// Errors a page request reports.
var (
	// ErrLimit is returned for a limit that is not a positive number.
	ErrLimit = fmt.Errorf("pagination: the limit is not a positive number")
	// ErrCursor is returned for a cursor that does not decode.
	ErrCursor = fmt.Errorf("pagination: the cursor is not one this service issued")
)

// Parse reads a page request from a query. A limit above the deployment's page size is clamped rather
// than refused: the caller asked for more than it may have, and the cap is the answer.
func Parse(values url.Values, pageSize int) (Page, error) {
	if pageSize <= 0 {
		return Page{}, fmt.Errorf("pagination: the page size is %d", pageSize)
	}
	page := Page{Limit: pageSize}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || limit <= 0 {
			return Page{}, fmt.Errorf("%w: %q", ErrLimit, raw)
		}
		if limit > pageSize {
			limit = pageSize
		}
		page.Limit = limit
	}
	if raw := values.Get("cursor"); raw != "" {
		after, err := Decode(raw)
		if err != nil {
			return Page{}, err
		}
		page.After = after
	}
	return page, nil
}

// Encode renders a cursor for the id given.
func Encode(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

// Decode reads a cursor.
func Decode(cursor string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(cursor))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCursor, err)
	}
	id := string(raw)
	if id == "" || strings.ContainsAny(id, "\x00\n\r,;'\"") {
		return "", fmt.Errorf("%w: %q", ErrCursor, id)
	}
	return id, nil
}

// Next returns the cursor of the next page: the id of the last row, or the empty string when the page
// carried fewer rows than it asked for.
func Next(page Page, rows int, lastID string) string {
	if rows < page.Limit || lastID == "" {
		return ""
	}
	return Encode(lastID)
}

// Slice returns the rows of a slice that belong to a page, given the index of a row's id. The rows are
// expected to be ordered by id, which is what makes the cursor a position rather than a guess.
func Slice[T any](rows []T, page Page, id func(T) string) []T {
	start := 0
	if page.After != "" {
		for i, row := range rows {
			if id(row) > page.After {
				start = i
				break
			}
			start = i + 1
		}
	}
	if start >= len(rows) {
		return nil
	}
	end := start + page.Limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

// Links renders the navigation links a listing response carries for a console page.
func Links(baseURL string, page Page, next string) map[string]string {
	links := make(map[string]string, 2)
	links["self"] = baseURL
	if next != "" {
		links["next"] = baseURL + "?limit=" + strconv.Itoa(page.Limit) + "&cursor=" + url.QueryEscape(next)
	}
	return links
}
