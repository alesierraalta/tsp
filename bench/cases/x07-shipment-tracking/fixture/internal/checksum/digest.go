// Package checksum is the shipment service's identity function. A payload, a report or a label is
// named by the SHA-256 of its bytes, so two readers that see the same digest see the same bytes and
// a service that is handed a different payload cannot pretend it was the one it promised.
package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrMismatch is reported when bytes do not carry the digest they were expected to carry.
var ErrMismatch = errors.New("checksum: digest mismatch")

// Size is the length of a digest in bytes.
const Size = sha256.Size

// Of returns the digest of data in lowercase hex.
func Of(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// OfString returns the digest of a string's bytes.
func OfString(value string) string { return Of([]byte(value)) }

// Valid reports whether raw is a lowercase hex SHA-256 digest. The check is a form check, not a
// comparison: it says a value could be a digest, never that it is the right one.
func Valid(raw string) bool {
	if len(raw) != hex.EncodedLen(Size) {
		return false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Verify compares the digest of data with want. The comparison is over the recomputed digest, so a
// caller never compares an untrusted string with a stored one byte by byte and never leaks a prefix
// of the right answer through the length of the check.
func Verify(data []byte, want string) error {
	if !Valid(want) {
		return fmt.Errorf("%w: %q is not a digest", ErrMismatch, want)
	}
	got := Of(data)
	if got != strings.ToLower(want) {
		return fmt.Errorf("%w: got %s want %s", ErrMismatch, got, want)
	}
	return nil
}

// Reader digests what a reader yields, up to a bound. It is how a label artifact is described
// without holding it in memory twice.
func Reader(r io.Reader, limit int64) (string, int64, error) {
	if limit <= 0 {
		return "", 0, fmt.Errorf("%w: bound %d", ErrMismatch, limit)
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(r, limit))
	if err != nil {
		return "", written, fmt.Errorf("checksum: read: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}
