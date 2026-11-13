// Package checksum is the archive's identity function for bytes. A manifest declares the digest a
// scan must have, the store keys its blobs by it, and a resume compares the two.
package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"strings"
)

// Algorithm is the only digest the manifest may name. A manifest that asks for another one is
// refused rather than silently verified with this one.
const Algorithm = "sha256"

// Size is the hex length of a digest this package produces and accepts.
const Size = sha256.Size * 2

// Sum returns the lowercase hex digest of data.
func Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// New returns a running digest for a caller that streams a file.
func New() hash.Hash { return sha256.New() }

// String reads a running digest as the hex value the manifest carries.
func String(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }

// Validate refuses a digest that is not exactly the hex of this algorithm. The check runs before a
// digest is compared to anything, so "sha256:" prefixes and uppercase forms never compare equal by
// accident.
func Validate(digest string) error {
	if len(digest) != Size {
		return fmt.Errorf("checksum: %q is not %d hex characters", digest, Size)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return fmt.Errorf("checksum: %q is not hex: %w", digest, err)
	}
	if strings.ToLower(digest) != digest {
		return fmt.Errorf("checksum: %q is uppercase", digest)
	}
	return nil
}

// Match reports whether data has the digest a manifest declared. A digest that does not validate
// is a mismatch, never a match: the comparison is only meaningful for a well-formed digest.
func Match(digest string, data []byte) bool {
	if Validate(digest) != nil {
		return false
	}
	return Sum(data) == digest
}

// Reader digests everything r yields, up to limit bytes, and refuses a stream longer than the limit
// rather than digesting a prefix of it.
func Reader(r io.Reader, limit int64) (string, int64, error) {
	h := New()
	n, err := io.Copy(h, io.LimitReader(r, limit+1))
	if err != nil {
		return "", n, fmt.Errorf("checksum: read: %w", err)
	}
	if n > limit {
		return "", n, fmt.Errorf("checksum: stream is longer than %d bytes", limit)
	}
	return String(h), n, nil
}

// Short is the digest prefix the audit trail prints next to an object. It is not a comparison: a
// decision always uses the full value.
func Short(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
