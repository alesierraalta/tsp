package store

import (
	"fmt"
	"strings"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/clock"
	"example.com/archive/internal/textutil"
)

// Validate refuses a record the store could not serve: a name with a separator, a kind the archive does
// not have, a page count a kind cannot carry, an instant that does not parse. Put calls it, so no caller
// can commit a record that a listing would refuse to render.
func (r Record) Validate() error {
	var problems []string
	if r.Name == "" {
		problems = append(problems, "name is empty")
	}
	if strings.ContainsAny(r.Name, `/\`) {
		problems = append(problems, "name carries a separator")
	}
	if r.Tenant == "" {
		problems = append(problems, "tenant is empty")
	}
	switch r.Kind {
	case "scan", "page", "sidecar", "index":
	default:
		problems = append(problems, fmt.Sprintf("kind %q is not scan, page, sidecar or index", r.Kind))
	}
	if r.Kind != "scan" && r.Pages != 0 {
		problems = append(problems, "only a scan carries pages")
	}
	if r.Bytes < 0 {
		problems = append(problems, "bytes is negative")
	}
	if r.Digest != "" {
		if err := checksum.Validate(r.Digest); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if r.Created != "" {
		if _, err := clock.ParseRFC3339(r.Created); err != nil {
			problems = append(problems, "created: "+err.Error())
		}
	}
	if r.Commit != "" {
		if _, err := clock.ParseRFC3339(r.Commit); err != nil {
			problems = append(problems, "committed: "+err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// Keys are the lookup keys a record is filed under in the index. The id is the primary key; the batch and
// the entry name are how a resume finds what already committed.
func (r Record) Keys() []string {
	keys := []string{"id:" + r.ID}
	if r.Batch != "" && r.Entry != "" {
		keys = append(keys, "entry:"+r.Batch+":"+r.Entry)
	}
	if r.Digest != "" {
		keys = append(keys, "digest:"+r.Digest)
	}
	return keys
}

// EntryKey is the key a resume looks a committed entry up by.
func EntryKey(batch, entry string) string { return "entry:" + batch + ":" + entry }

// Describe renders a record for a log line or an audit detail.
func (r Record) Describe() string {
	return textutil.Sentence(
		r.ID,
		r.Kind,
		r.Name,
		clock.Stamp(r.CommittedAt()),
		textutil.Bytes(r.Bytes),
		fmt.Sprintf("%d pages", r.Pages),
	)
}

// Preview is the one-line label a listing prints under the title.
func (r Record) Preview() string {
	parts := []string{r.Kind}
	if r.Pages > 0 {
		parts = append(parts, textutil.Plural(r.Pages, "page", "pages"))
	}
	parts = append(parts, textutil.Bytes(r.Bytes))
	if len(r.Tags) > 0 {
		parts = append(parts, strings.Join(r.Tags, "/"))
	}
	return strings.Join(parts, " · ")
}

// SameContent reports whether two records carry the same bytes: the digest decides, and a record without
// a digest never matches, because an unknown digest is not evidence of shared content.
func (r Record) SameContent(other Record) bool {
	if r.Digest == "" || other.Digest == "" {
		return false
	}
	return r.Digest == other.Digest
}

// WithTags returns the record with its tags replaced, keeping the rest untouched. The report page uses it
// to render a tag set without mutating the stored record.
func (r Record) WithTags(tags []string) Record {
	r.Tags = append([]string(nil), tags...)
	return r
}
