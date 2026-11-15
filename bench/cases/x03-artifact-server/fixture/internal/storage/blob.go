package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Blob is one content-addressed file: its digest, its size and where it sits under the root.
type Blob struct {
	Digest string
	Bytes  int64
	Path   string
}

// Digest returns the SHA-256 of a body, which is the tree's identity function: two files with the same
// bytes have one digest and one path, so a duplicate upload costs no space and a corrupted file is
// detectable from its name.
func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// DigestFile returns the SHA-256 of a file's content.
func DigestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("storage: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("storage: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// BlobName returns the tree path of a digest: two characters of fan-out and the digest itself, so a
// directory never holds more files than a filesystem lists in one read.
func BlobName(digest string) (string, error) {
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return "", fmt.Errorf("%w: %q is not a digest", ErrInvalid, digest)
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("%w: %q is not a digest", ErrInvalid, digest)
		}
	}
	return filepath.Join("blobs", digest[:2], digest), nil
}

// PutBlob stores a body under its digest. A body that is already stored is not written again: the
// second write would be the same bytes at the same path.
func PutBlob(root string, body []byte) (Blob, error) {
	digest := Digest(body)
	name, err := BlobName(digest)
	if err != nil {
		return Blob{}, err
	}
	if Exists(root, name) {
		size, err := Size(root, name)
		if err != nil {
			return Blob{}, err
		}
		return Blob{Digest: digest, Bytes: size, Path: name}, nil
	}
	if _, err := Write(root, name, body, 0o644); err != nil {
		return Blob{}, err
	}
	return Blob{Digest: digest, Bytes: int64(len(body)), Path: name}, nil
}

// ReadBlob returns the body a digest names, verifying that the bytes still hash to it.
func ReadBlob(root, digest string) ([]byte, error) {
	name, err := BlobName(digest)
	if err != nil {
		return nil, err
	}
	body, err := Read(root, name)
	if err != nil {
		return nil, err
	}
	if Digest(body) != digest {
		return nil, fmt.Errorf("storage: blob %q does not hash to its name", digest)
	}
	return body, nil
}

// RemoveBlob deletes a blob by digest.
func RemoveBlob(root, digest string) error {
	name, err := BlobName(digest)
	if err != nil {
		return err
	}
	return Remove(root, name)
}

// VerifyBlob reports whether a stored blob still matches its digest.
func VerifyBlob(root, digest string) error {
	name, err := BlobName(digest)
	if err != nil {
		return err
	}
	path, err := Join(root, name)
	if err != nil {
		return err
	}
	found, err := DigestFile(path)
	if err != nil {
		return err
	}
	if found != digest {
		return fmt.Errorf("storage: blob %q hashes to %q", digest, found)
	}
	return nil
}

// BlobPath returns the tree path of a digest without checking that it exists.
func BlobPath(root, digest string) (string, error) {
	name, err := BlobName(digest)
	if err != nil {
		return "", err
	}
	return Join(root, name)
}
