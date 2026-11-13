package vault

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Attachment names are the names the upload endpoint accepts: they live under "attachments/" in the
// tree, they are flat (no directory part), and their extension must be one the service knows. The
// object tree itself is reached only through Resolve.
const attachPrefix = "attachments"

// Attachment is one stored attachment.
type Attachment struct {
	Name string `json:"name"`
	Key  string `json:"key"`
	Size int64  `json:"size"`
}

// extensions the attachment endpoint accepts. A name outside the list is refused before it reaches
// the tree, so the tree never carries a file the service cannot serve.
var attachExt = map[string]bool{
	".png": true, ".jpg": true, ".pdf": true, ".txt": true, ".csv": true,
}

// AttachmentPath is the object name of one attachment: the prefix plus the flat name.
func AttachmentPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty attachment name", ErrInvalid)
	}
	if strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: attachment %q is not a flat name", ErrInvalid, name)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("%w: attachment %q is not a name", ErrInvalid, name)
	}
	ext := strings.ToLower(path.Ext(name))
	if !attachExt[ext] {
		return "", fmt.Errorf("%w: attachment extension %q is not accepted", ErrInvalid, ext)
	}
	return attachPrefix + "/" + name, nil
}

// ReadAttachment reads one attachment through the vault's containment check.
func (v *Vault) ReadAttachment(name string) ([]byte, error) {
	object, err := AttachmentPath(name)
	if err != nil {
		return nil, err
	}
	return v.Read(object)
}

// WriteAttachment stores one attachment and returns its metadata.
func (v *Vault) WriteAttachment(name string, data []byte) (Attachment, error) {
	object, err := AttachmentPath(name)
	if err != nil {
		return Attachment{}, err
	}
	if err := v.Write(object, data); err != nil {
		return Attachment{}, err
	}
	k, err := v.Key(object)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{Name: name, Key: k, Size: int64(len(data))}, nil
}

// Attachments lists the attachment objects of the tree, as metadata.
func (v *Vault) Attachments() ([]Attachment, error) {
	keys, err := v.List(attachPrefix)
	if err != nil {
		if errors.Is(err, ErrNoObject) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Attachment, 0, len(keys))
	for _, k := range keys {
		name := path.Base(k)
		size, _, err := v.Stat(k)
		if err != nil {
			continue
		}
		out = append(out, Attachment{Name: name, Key: k, Size: size})
	}
	return out, nil
}

// ParseRange parses the "start-end" body the download endpoint accepts for a byte range header. A
// malformed range is refused instead of clamped, because a clamped read would hand out bytes the
// caller did not ask for.
func ParseRange(header string, size int64) (start, end int64, err error) {
	if header == "" {
		return 0, size, nil
	}
	if size < 0 {
		return 0, 0, fmt.Errorf("%w: negative size", ErrInvalid)
	}
	parts := strings.SplitN(strings.TrimSpace(header), "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("%w: range %q", ErrInvalid, header)
	}
	start, err = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 0 || start > size {
		return 0, 0, fmt.Errorf("%w: range start %q", ErrInvalid, parts[0])
	}
	end = size
	if s := strings.TrimSpace(parts[1]); s != "" {
		end, err = strconv.ParseInt(s, 10, 64)
		if err != nil || end < start || end > size {
			return 0, 0, fmt.Errorf("%w: range end %q", ErrInvalid, parts[1])
		}
	}
	return start, end, nil
}
