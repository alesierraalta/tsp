package intake

import (
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"example.com/archive/internal/textutil"
	"fmt"
	"strconv"
	"strings"
)

// extractPage strips the PGM header of one page and renders the payload as a text line. The archive
// does not run OCR itself: the scanner writes the recognised text as the payload of the PGM, and this
// is where the pipeline reads it.
func extractPage(p staging.Page, body []byte) string {
	payload := pgmPayload(body)
	line := textutil.Collapse(string(payload))
	if line == "" {
		return fmt.Sprintf("[page %03d] empty\n", p.Number)
	}
	return fmt.Sprintf("[page %03d] %s\n", p.Number, line)
}

// pgmPayload returns everything after the PGM header. A body that does not start with a PGM magic
// number is returned whole: the pipeline skips a header it does not recognise rather than dropping the
// page.
func pgmPayload(body []byte) []byte {
	header, rest, ok := splitPGMHeader(body)
	if !ok || len(header) == 0 {
		return body
	}
	return rest
}

// splitPGMHeader finds the end of the two-line netpbm header (magic and dimensions, then the maximum
// value line) and returns the header bytes and the payload after them.
func splitPGMHeader(body []byte) (header, rest []byte, ok bool) {
	if len(body) < 3 || body[0] != 'P' || (body[1] != '5' && body[1] != '2') {
		return nil, body, false
	}
	seen := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '\n' {
			continue
		}
		seen++
		if seen == 3 {
			return body[:i+1], body[i+1:], true
		}
	}
	return nil, body, false
}

// extractSummary is the first line of the extracted text, the value the listing shows as a preview.
func extractSummary(text string) string {
	first := text
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = first[:idx]
	}
	return textutil.Label(textutil.Collapse(first))
}

// textLength is how many bytes of text the report prints, capped so a page stays one page.
func textLength(text string) string { return strconv.Itoa(len(text)) }

// DeriveTitle is the title an entry gets when the sender did not write one: the entry name with its
// separators turned into spaces, which reads as a label rather than as a file name.
func DeriveTitle(e manifest.Entry) string {
	base := strings.TrimSuffix(e.Name, "."+strings.TrimPrefix(extOf(e.Name), "."))
	replaced := strings.NewReplacer("-", " ", "_", " ").Replace(base)
	return textutil.Collapse(replaced)
}

// extOf is the extension of an entry name without the dot.
func extOf(name string) string {
	if idx := strings.LastIndexByte(name, '.'); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return ""
}
