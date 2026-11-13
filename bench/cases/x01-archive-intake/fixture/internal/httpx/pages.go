package httpx

import (
	"fmt"
	"net/url"
	"strconv"
)

// Page is one page of a listing: which slice of the ordered rows it covers and how many rows exist.
type Page struct {
	Number int
	Size   int
	Total  int
}

// PageFor builds the page a request asked for. A page number beyond the last page is clamped to the last
// page rather than refused: a client that follows a stale link gets the final page, not an error.
func PageFor(number, size, total int) Page {
	if size <= 0 {
		size = 25
	}
	last := LastPage(size, total)
	if number < 1 {
		number = 1
	}
	if last > 0 && number > last {
		number = last
	}
	return Page{Number: number, Size: size, Total: total}
}

// LastPage is the number of the last page of total rows, or 1 when there are none.
func LastPage(size, total int) int {
	if size <= 0 || total <= 0 {
		return 1
	}
	return (total + size - 1) / size
}

// Offset is the index of the page's first row.
func (p Page) Offset() int {
	if p.Number < 1 {
		return 0
	}
	return (p.Number - 1) * p.Size
}

// Slice returns the part of rows the page covers. A caller that already sliced its rows passes the page's
// offset and size to its own query; this helper is for the pages that hold their rows in memory.
func (p Page) Slice(rows []string) []string {
	start := p.Offset()
	if start >= len(rows) {
		return nil
	}
	end := start + p.Size
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

// HasPrev and HasNext report whether a neighbouring page exists.
func (p Page) HasPrev() bool { return p.Number > 1 }
func (p Page) HasNext() bool { return p.Number < LastPage(p.Size, p.Total) }

// Link builds the URL of another page of the same listing: the path, the caller's own query, and the
// replaced page number. The query is re-encoded from parsed values, so a name the caller passed cannot
// inject a parameter the page did not carry.
func (p Page) Link(path string, values url.Values, number int) string {
	copied := url.Values{}
	for key, list := range values {
		if key == "page" {
			continue
		}
		copied[key] = append([]string(nil), list...)
	}
	if number > 1 {
		copied.Set("page", strconv.Itoa(number))
	}
	encoded := copied.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}

// Describe renders the page line the listing prints: "page 2 of 5, 97 rows".
func (p Page) Describe() string {
	return fmt.Sprintf("page %d of %d, %d rows", p.Number, LastPage(p.Size, p.Total), p.Total)
}
