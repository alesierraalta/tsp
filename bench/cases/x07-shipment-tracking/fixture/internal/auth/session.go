package auth

import (
	"fmt"
	"sync"
	"time"
)

// Sessions tracks the tokens a deployment has seen, so an operator can revoke one before it expires.
// A session is keyed by the token's signature-derived id, never by the token itself: the store keeps
// no credential at rest.
type Sessions struct {
	mu     sync.Mutex
	ttl    time.Duration
	max    int
	seen   map[string]Session
	order  []string
	now    func() time.Time
	issued int
}

// Session is one recorded presentation of a token.
type Session struct {
	ID      string
	Subject string
	Tenant  string
	First   time.Time
	Last    time.Time
	Count   int
}

// NewSessions returns a session table with a lifetime and a cap. The cap is a hard ceiling: a
// deployment cannot be configured to remember more presentations than it can hold, because a table
// that grows without a bound is a memory leak a caller controls.
func NewSessions(ttl time.Duration, max int) (*Sessions, error) {
	if ttl <= 0 || ttl > 24*time.Hour {
		return nil, fmt.Errorf("auth: session lifetime %s", ttl)
	}
	if max < 1 || max > 100000 {
		return nil, fmt.Errorf("auth: session cap %d", max)
	}
	return &Sessions{ttl: ttl, max: max, seen: map[string]Session{}, now: time.Now}, nil
}

// SetClock replaces the clock a session reads.
func (s *Sessions) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// Record notes a presentation of a verified token and returns the session. The id is derived from the
// claims, so two presentations of the same token land on one session and two tokens for the same
// subject land on two.
func (s *Sessions) Record(tokenID string, claims Claims) Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	s.expireLocked(now)
	session, ok := s.seen[tokenID]
	if !ok {
		if len(s.seen) >= s.max {
			// The table is full of live sessions: the oldest live one is dropped rather than letting the
			// table grow. A revoked session that is dropped can be presented again, which is why the cap is
			// a deployment ceiling and not a request value.
			s.dropOldestLocked()
		}
		session = Session{ID: tokenID, Subject: claims.Subject, Tenant: claims.Tenant, First: now}
		s.order = append(s.order, tokenID)
	}
	session.Last = now
	session.Count++
	s.seen[tokenID] = session
	return session
}

// Lookup returns a recorded session.
func (s *Sessions) Lookup(tokenID string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.seen[tokenID]
	return session, ok
}

// Revoke forgets a session; a token of a revoked session is still verified by signature, and the
// handler refuses it because the session is gone.
func (s *Sessions) Revoke(tokenID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[tokenID]; !ok {
		return false
	}
	delete(s.seen, tokenID)
	for i, id := range s.order {
		if id == tokenID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return true
}

// Len reports how many live sessions the table holds.
func (s *Sessions) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.now().UTC())
	return len(s.seen)
}

func (s *Sessions) expireLocked(now time.Time) {
	horizon := now.Add(-s.ttl)
	kept := s.order[:0]
	for _, id := range s.order {
		session, ok := s.seen[id]
		if !ok {
			continue
		}
		if session.Last.Before(horizon) {
			delete(s.seen, id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
}

func (s *Sessions) dropOldestLocked() {
	for len(s.order) > 0 {
		id := s.order[0]
		s.order = s.order[1:]
		if _, ok := s.seen[id]; ok {
			delete(s.seen, id)
			return
		}
	}
}
