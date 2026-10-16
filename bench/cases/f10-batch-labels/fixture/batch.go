package batch

import "encoding/binary"

func VisitIDs(ids []uint64, visit func([]byte)) {
	for _, id := range ids {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, id)
		visit(buf)
	}
}

type Labels struct {
	limit   int
	entries map[string]string
	order   []string
	render  func(string) string
}

func NewLabels(limit int, render func(string) string) *Labels {
	return &Labels{limit: limit, entries: make(map[string]string), render: render}
}

func (l *Labels) Label(id string) string {
	if value, ok := l.entries[id]; ok {
		return value
	}
	value := l.render(id)
	l.entries[id] = value
	l.order = append(l.order, id)
	return value
}

func (l *Labels) Size() int { return len(l.entries) }
