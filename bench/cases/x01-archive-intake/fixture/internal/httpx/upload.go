package httpx

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"example.com/archive/internal/manifest"
)

// Errors a multipart delivery produces.
var (
	ErrNoManifest = errors.New("httpx: no manifest part")
	ErrPartLimit  = errors.New("httpx: too many parts")
)

// Delivery is one multipart batch upload: the manifest and the bytes of every entry.
type Delivery struct {
	Manifest manifest.Manifest
	Upload   Upload
}

// Upload is the payload a delivery carries, keyed by entry name. It mirrors what the intake pipeline
// takes, so the handler does no translation of its own.
type Upload struct {
	Containers map[string][]byte
	Pages      map[string]map[int][]byte
	Sidecars   map[string][]byte
}

// ReadDelivery reads a multipart delivery under the given caps. The field convention is the one the
// scanner's uploader uses:
//
//	manifest            the JSON manifest
//	container:<entry>   the entry's container
//	page:<entry>:<n>    page n of a scan, 1-based
//	sidecar:<entry>     a text sidecar the scanner wrote itself
//
// A part whose name does not follow the convention is refused: the service does not guess what an
// unlabelled part is.
func ReadDelivery(r *http.Request, limits Limits) (Delivery, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return Delivery{}, fmt.Errorf("httpx: content type: %w", err)
	}
	if mediaType != "multipart/form-data" {
		return Delivery{}, fmt.Errorf("httpx: content type %q is not multipart/form-data", mediaType)
	}
	reader, err := r.MultipartReader()
	if err != nil {
		return Delivery{}, fmt.Errorf("httpx: read multipart: %w", err)
	}
	delivery := Delivery{Upload: Upload{
		Containers: map[string][]byte{},
		Pages:      map[string]map[int][]byte{},
		Sidecars:   map[string][]byte{},
	}}
	parts := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Delivery{}, fmt.Errorf("httpx: next part: %w", err)
		}
		parts++
		if parts > limits.MaxParts {
			part.Close()
			return Delivery{}, fmt.Errorf("%w: more than %d", ErrPartLimit, limits.MaxParts)
		}
		if err := readPart(part, limits, &delivery); err != nil {
			part.Close()
			return Delivery{}, err
		}
		if err := part.Close(); err != nil {
			return Delivery{}, fmt.Errorf("httpx: close part: %w", err)
		}
	}
	if delivery.Manifest.Tenant == "" {
		return Delivery{}, fmt.Errorf("%w", ErrNoManifest)
	}
	return delivery, nil
}

// Limits are the caps a delivery is read under.
type Limits struct {
	MaxBodyBytes int64
	MaxUpload    int64
	MaxParts     int
}

// readPart reads one part and files it in the delivery. A part is read under the body cap, and the
// entry name is checked by the manifest, so a part can never name an entry the manifest did not
// declare.
func readPart(part *multipart.Part, limits Limits, delivery *Delivery) error {
	name := part.FormName()
	switch {
	case name == "manifest":
		body, err := ReadAll(part, limits.MaxBodyBytes)
		if err != nil {
			return err
		}
		m, err := manifest.Decode(body, manifest.Limits{
			MaxUpload:    limits.MaxUpload,
			MaxEntries:   limits.MaxParts,
			MaxPages:     limits.MaxParts,
			MaxBodyBytes: limits.MaxBodyBytes,
			MaxNameBytes: 64,
		})
		if err != nil {
			return err
		}
		delivery.Manifest = m
		return nil
	case strings.HasPrefix(name, "container:"):
		entry := strings.TrimPrefix(name, "container:")
		if entry == "" {
			return fmt.Errorf("httpx: a container part names no entry")
		}
		body, err := ReadAll(part, limits.MaxUpload)
		if err != nil {
			return err
		}
		delivery.Upload.Containers[entry] = body
		return nil
	case strings.HasPrefix(name, "sidecar:"):
		entry := strings.TrimPrefix(name, "sidecar:")
		body, err := ReadAll(part, limits.MaxUpload)
		if err != nil {
			return err
		}
		delivery.Upload.Sidecars[entry] = body
		return nil
	case strings.HasPrefix(name, "page:"):
		rest := strings.TrimPrefix(name, "page:")
		entry, number, ok := strings.Cut(rest, ":")
		if !ok || entry == "" {
			return fmt.Errorf("httpx: page part %q does not name an entry and a page", name)
		}
		page, err := parsePageNumber(number)
		if err != nil {
			return err
		}
		body, err := ReadAll(part, limits.MaxUpload)
		if err != nil {
			return err
		}
		if delivery.Upload.Pages[entry] == nil {
			delivery.Upload.Pages[entry] = map[int][]byte{}
		}
		delivery.Upload.Pages[entry][page] = body
		return nil
	default:
		return fmt.Errorf("httpx: part %q is not manifest, container:<entry>, page:<entry>:<n> or sidecar:<entry>", name)
	}
}

// parsePageNumber reads a 1-based page number from a part name.
func parsePageNumber(raw string) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("httpx: page part names no page number")
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("httpx: page number %q is not a number", raw)
		}
		n = n*10 + int(r-'0')
		if n > 1_000_000 {
			return 0, fmt.Errorf("httpx: page number %q is out of range", raw)
		}
	}
	if n < 1 {
		return 0, fmt.Errorf("httpx: page number %q is not positive", raw)
	}
	return n, nil
}
