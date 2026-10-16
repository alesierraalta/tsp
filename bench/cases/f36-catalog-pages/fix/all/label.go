package catalog

import (
	"errors"
	"strings"
	"unicode/utf8"
)

func Label(title string, budget int) (string, error) {
	if budget < 0 {
		return "", errors.New("negative byte budget")
	}
	if !utf8.ValidString(title) {
		return "", errors.New("invalid UTF-8")
	}
	title = strings.TrimSpace(title)
	if len(title) <= budget {
		return title, nil
	}
	end := budget
	for end > 0 && !utf8.RuneStart(title[end]) {
		end--
	}
	return title[:end], nil
}

func GroupLabel(r Record) string {
	if r.Group == "" {
		return "ungrouped"
	}
	return r.Group
}

func FullLabel(r Record) string {
	return r.ID + ": " + r.Title
}

func JoinLabels(records []Record) string {
	labels := make([]string, len(records))
	for i, r := range records {
		labels[i] = FullLabel(r)
	}
	return strings.Join(labels, "\n")
}
