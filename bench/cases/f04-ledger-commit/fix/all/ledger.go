package ledger

import "sync"

type Ledger struct {
	mu      sync.Mutex
	credits int
	claimed map[string]bool
	persist func(string, int) error
}

func New(persist func(string, int) error) *Ledger {
	return &Ledger{claimed: make(map[string]bool), persist: persist}
}

func (l *Ledger) Claim(id string) (bool, error) {
	l.mu.Lock()
	exists := l.claimed[id]
	l.mu.Unlock()
	if exists {
		return false, nil
	}
	if err := l.persist(id, 0); err != nil {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.claimed[id] {
		l.claimed[id] = true
		return true, nil
	}
	return false, nil
}

func (l *Ledger) Add(delta int) error {
	if err := l.persist("credit", delta); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.credits += delta
	return nil
}

func (l *Ledger) Balance() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.credits
}
