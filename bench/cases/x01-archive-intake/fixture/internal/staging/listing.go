package staging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"example.com/archive/internal/storepath"
)

// FileInfo is one file of a staged batch, as the operator page lists it: the key, the size and the role
// the extension gives it.
type FileInfo struct {
	Key   string
	Bytes int64
	Role  string
}

// Listing is what one staged batch holds.
type Listing struct {
	Batch string
	Files []FileInfo
	Bytes int64
}

// List walks the batch tree and describes every file. It reads the tree through the layout's own paths
// and refuses a file that is not regular: a device or a fifo in the tree is a delivery the archive did
// not make.
func (l Layout) List() (Listing, error) {
	listing := Listing{Batch: l.batch}
	err := filepath.Walk(l.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staging: %s is not a regular file", path)
		}
		key, err := storepath.Key(l.dir, path)
		if err != nil {
			return err
		}
		listing.Files = append(listing.Files, FileInfo{Key: key, Bytes: info.Size(), Role: storepath.Kind(key)})
		listing.Bytes += info.Size()
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return Listing{}, fmt.Errorf("%w: %s", ErrMissing, l.batch)
		}
		return Listing{}, fmt.Errorf("staging: list %s: %w", l.batch, err)
	}
	sort.Slice(listing.Files, func(i, j int) bool { return listing.Files[i].Key < listing.Files[j].Key })
	return listing, nil
}

// Roles counts the files of a listing by the role their extension gives them.
func (l Listing) Roles() map[string]int {
	roles := map[string]int{}
	for _, file := range l.Files {
		roles[file.Role]++
	}
	return roles
}

// Entries lists the entry names the listing holds, read from the scans/ and sidecars/ prefixes. It is a
// read of the tree, not of the manifest: the two disagree exactly when a delivery is incomplete, and the
// console shows both.
func (l Listing) Entries() []string {
	seen := map[string]bool{}
	var names []string
	for _, file := range l.Files {
		parts := strings.Split(file.Key, "/")
		if len(parts) < 3 {
			continue
		}
		switch parts[0] {
		case "scans", "sidecars":
			name := parts[1]
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Describe renders the listing as the block the console prints: the batch, its size and its roles.
func (l Listing) Describe() []string {
	lines := []string{fmt.Sprintf("batch %s: %d files, %d bytes", l.Batch, len(l.Files), l.Bytes)}
	roles := l.Roles()
	for _, role := range []string{"scan", "page", "sidecar", "manifest", "index", "other"} {
		if roles[role] == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d %s", roles[role], role))
	}
	for _, file := range l.Files {
		lines = append(lines, fmt.Sprintf("%s %d bytes %s", file.Key, file.Bytes, file.Role))
	}
	return lines
}
