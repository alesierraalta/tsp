package intake

import (
	"fmt"
	"sort"
	"strings"

	"example.com/archive/internal/manifest"
	"example.com/archive/internal/storepath"
	"example.com/archive/internal/textutil"
)

// Classification is what the pipeline decides about an entry before it reads a byte of it: the kind the
// name implies, the title a page prints, the tags the index carries and whether the entry repeats another.
type Classification struct {
	Name     string
	Kind     string
	Title    string
	Tags     []string
	PageHint int
	Problems []string
}

// Classify decides one entry's classification. It reads the manifest value only; the container is
// classified by its name and the kind the sender declared, which is what the accept step already
// validated. A name that looks like a page image of another entry is reported, because a delivery that
// ships a page as a top-level object is usually a packaging mistake.
func Classify(e manifest.Entry, siblings []manifest.Entry) Classification {
	kind := e.KindOf()
	if normalized := manifest.NormalizeKind(kind); normalized != "" {
		kind = normalized
	}
	c := Classification{
		Name:     e.Name,
		Kind:     kind,
		Title:    entryTitle(e),
		Tags:     deriveTags(e),
		PageHint: e.Pages,
	}
	if kind != manifest.KindScan && e.Pages > 0 {
		c.Problems = append(c.Problems, "a non-scan entry declares pages")
	}
	if strings.HasPrefix(storepath.Stem(e.Name), "page-") && kind == manifest.KindScan {
		c.Problems = append(c.Problems, "a scan named like a page image")
	}
	for _, sibling := range siblings {
		if sibling.Name == e.Name {
			continue
		}
		if storepath.Stem(sibling.Name) == storepath.Stem(e.Name) {
			c.Problems = append(c.Problems, "shares its stem with "+sibling.Name)
		}
	}
	return c
}

// ClassifyAll classifies every entry of a manifest, in declared order.
func ClassifyAll(m manifest.Manifest) []Classification {
	out := make([]Classification, 0, len(m.Entries))
	for _, e := range m.Entries {
		out = append(out, Classify(e, m.Entries))
	}
	return out
}

// KindCounts renders the classification counts as the report's one line.
func KindCounts(classes []Classification) string {
	counts := map[string]int{}
	for _, c := range classes {
		counts[c.Kind]++
	}
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", counts[kind], kind))
	}
	return strings.Join(parts, ", ")
}

// Warnings lists the classifications that carry a problem, as the report's warning block.
func Warnings(classes []Classification) []string {
	var out []string
	for _, c := range classes {
		for _, problem := range c.Problems {
			out = append(out, c.Name+": "+problem)
		}
	}
	return out
}

// Vocabulary is the tag vocabulary the pipeline adds to a delivery: the kind, the source class and the
// tenant's own labels. It is a function rather than a constant so the report can print it.
func Vocabulary(class Classification, source string) []string {
	tags := append([]string{class.Kind}, class.Tags...)
	if sourceClass := manifest.SourceClass(source); sourceClass != "none" {
		tags = append(tags, "source-"+sourceClass)
	}
	return tags
}

// Describe renders one classification for a log line.
func (c Classification) Describe() string {
	return textutil.Sentence(c.Name, c.Kind, c.Title, fmt.Sprintf("%d pages", c.PageHint), strings.Join(c.Problems, ", "))
}
