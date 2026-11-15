// Package archive reads the tar streams the import pipeline accepts and decides where an entry may
// be written.
package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
)

// Errors a reader reports.
var (
	ErrMalformed = errors.New("archive: malformed stream")
	ErrTooLarge  = errors.New("archive: entry above the size limit")
	ErrTooMany   = errors.New("archive: entry count above the limit")
)

// Entry is one file of a stream, already read into memory. The body is a value so a caller cannot
// keep the stream open behind the pipeline's back.
type Entry struct {
	Name string
	Mode int64
	Body []byte
}

// Limits bound a stream before it is decoded.
type Limits struct {
	MaxEntries int
	MaxEntry   int64
	MaxTotal   int64
}

// DefaultLimits are the bounds a deployment runs with.
func DefaultLimits() Limits {
	return Limits{MaxEntries: 200, MaxEntry: 1 << 20, MaxTotal: 8 << 20}
}

// valid reports whether the limits can be applied.
func (l Limits) valid() bool {
	return l.MaxEntries > 0 && l.MaxEntry > 0 && l.MaxTotal > 0
}

// Read drains a tar stream into entries, enforcing the limits as it goes: the count is checked
// before an entry is allocated and the size before its body is read, so a stream cannot make the
// service hold more than the limits allow.
func Read(r io.Reader, limits Limits) ([]Entry, error) {
	if !limits.valid() {
		return nil, fmt.Errorf("archive: limits are not set")
	}
	tr := tar.NewReader(r)
	var entries []Entry
	var total int64
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		if len(entries) >= limits.MaxEntries {
			return nil, fmt.Errorf("%w: %d entries", ErrTooMany, len(entries))
		}
		if header.Size > limits.MaxEntry {
			return nil, fmt.Errorf("%w: %q is %d bytes", ErrTooLarge, header.Name, header.Size)
		}
		if total+header.Size > limits.MaxTotal {
			return nil, fmt.Errorf("%w: the stream is above %d bytes", ErrTooLarge, limits.MaxTotal)
		}
		body := make([]byte, 0, header.Size)
		buf := make([]byte, 4096)
		for int64(len(body)) < header.Size {
			n, err := tr.Read(buf)
			if n > 0 {
				body = append(body, buf[:n]...)
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
			}
		}
		total += int64(len(body))
		entries = append(entries, Entry{Name: header.Name, Mode: header.Mode, Body: body})
	}
	return entries, nil
}

// Kind classifies an entry by its first directory. A stream the importer does not understand is
// refused by the importer, which is where the pipeline's kinds are declared.
func Kind(name string) string {
	for i := 0; i < len(name); i++ {
		if name[i] == '/' {
			return name[:i]
		}
	}
	return ""
}
