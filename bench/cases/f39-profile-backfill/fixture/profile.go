package backfill

import (
	"errors"
	"sort"
	"strings"
)

var ErrBusy = errors.New("job capacity reached")

type Profile struct {
	ID       string
	Handle   string
	Name     string
	Team     string
	State    string
	Attempts int
}

type Card struct {
	ID    string
	Title string
	Team  string
	Terms []string
}

func BuildCard(p Profile) Card {
	title := strings.TrimSpace(p.Name)
	if title == "" {
		title = p.Handle
	}
	seen := make(map[string]bool)
	for _, term := range strings.Fields(strings.ToLower(title + " " + p.Handle)) {
		seen[term] = true
	}
	terms := make([]string, 0, len(seen))
	for term := range seen {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return Card{ID: p.ID, Title: title, Team: p.Team, Terms: terms}
}

func copyCard(c Card) Card {
	c.Terms = append([]string(nil), c.Terms...)
	return c
}
