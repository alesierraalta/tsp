package backfill

import (
	"context"
	"sort"
	"sync"
)

type Sink interface {
	Put(context.Context, Card) error
}

type Index struct {
	mu    sync.Mutex
	cards map[string]Card
}

func NewIndex() *Index {
	return &Index{cards: make(map[string]Card)}
}

func (i *Index) Put(ctx context.Context, c Card) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cards[c.ID] = copyCard(c)
	return nil
}

func (i *Index) Cards() []Card {
	i.mu.Lock()
	defer i.mu.Unlock()
	cards := make([]Card, 0, len(i.cards))
	for _, c := range i.cards {
		cards = append(cards, copyCard(c))
	}
	sort.Slice(cards, func(a, b int) bool { return cards[a].ID < cards[b].ID })
	return cards
}
