package intake

import (
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/staging"
	"fmt"
	"strings"
)

// extractEntry turns the pages of an entry into the text the store keeps. A scan yields one block per
// page, a sidecar yields its own bytes, and an index yields the names it lists. Every page is read
// through the layout's page reader, so extraction never opens a file the containment rule refused.
func (r *Runner) extractEntry(layout staging.Layout, e manifest.Entry, pages []staging.Page) (string, error) {
	switch e.KindOf() {
	case manifest.KindScan:
		var b strings.Builder
		for _, p := range pages {
			body, err := layout.OpenPage(e, p)
			if err != nil {
				return "", fmt.Errorf("intake: extract %q page %d: %w", e.Name, p.Number, err)
			}
			b.WriteString(extractPage(p, body))
		}
		return b.String(), nil
	case manifest.KindSidecar, manifest.KindIndex:
		body, err := layout.ReadSidecar(e)
		if err != nil {
			if container, readErr := layout.ReadEntry(e); readErr == nil {
				return string(container), nil
			}
			return "", fmt.Errorf("intake: extract %q: %w", e.Name, err)
		}
		return string(body), nil
	default:
		return "", fmt.Errorf("intake: %q has no extraction rule for kind %q", e.Name, e.KindOf())
	}
}
