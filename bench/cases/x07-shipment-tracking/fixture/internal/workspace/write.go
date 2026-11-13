package workspace

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write stores an object, creating its directory, and returns the path it wrote. The bytes go to a
// temporary file in the object's own directory and are renamed over the target, so a reader never
// sees half a statement: either the previous object or the new one is there.
func (w *Workspace) Write(name string, data []byte, mode fs.FileMode) (string, error) {
	target, err := w.Resolve(name)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("workspace: prepare %q: %w", name, err)
	}
	temp, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return "", fmt.Errorf("workspace: write %q: %w", name, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return "", fmt.Errorf("workspace: write %q: %w", name, err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("workspace: sync %q: %w", name, err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("workspace: close %q: %w", name, err)
	}
	if err := os.Chmod(tempName, mode); err != nil {
		_ = os.Remove(tempName)
		return "", fmt.Errorf("workspace: mode %q: %w", name, err)
	}
	if err := os.Rename(tempName, target); err != nil {
		_ = os.Remove(tempName)
		return "", fmt.Errorf("workspace: replace %q: %w", name, err)
	}
	return target, nil
}

// Read returns the bytes of a regular file under the root, up to limit bytes. The object is examined
// with Lstat before it is opened, so a symbolic link is refused rather than followed: a link can point
// anywhere, and a name that resolves inside the tree must name an object inside the tree. An object
// that is not a regular file (a directory, a device, a fifo) is refused for the same reason — reading
// it would mean reading something the service did not write.
func (w *Workspace) Read(name string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: bound %d", ErrTooLarge, limit)
	}
	target, err := w.Resolve(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q", ErrNoObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: stat %q: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %q is a symbolic link", ErrNotRegular, name)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", ErrNotRegular, name)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%w: %q is %d bytes", ErrTooLarge, name, info.Size())
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("workspace: open %q: %w", name, err)
	}
	defer file.Close()
	// The bound is applied to the read as well, so an object that grew between Lstat and Open cannot
	// hand back more than the caller asked for.
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, fmt.Errorf("workspace: read %q: %w", name, err)
	}
	return data, nil
}

// Remove deletes an object under the root. It refuses a name it cannot resolve, so a caller cannot
// delete through a path that left the tree, and it removes only a regular file: a directory named like
// an object stays.
func (w *Workspace) Remove(name string) error {
	target, err := w.Resolve(name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("workspace: stat %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q", ErrNotRegular, name)
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("workspace: remove %q: %w", name, err)
	}
	return nil
}

// Size reports the size of an object under the root, refusing a name that is not a regular file.
func (w *Workspace) Size(name string) (int64, error) {
	target, err := w.Resolve(name)
	if err != nil {
		return 0, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("%w: %q", ErrNoObject, name)
	}
	if err != nil {
		return 0, fmt.Errorf("workspace: stat %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("%w: %q", ErrNotRegular, name)
	}
	return info.Size(), nil
}
