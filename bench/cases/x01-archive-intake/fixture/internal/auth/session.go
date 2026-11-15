package auth

import (
	"fmt"
	"sync"
	"time"

	"example.com/archive/internal/clock"
	"example.com/archive/internal/idgen"
)

// Session is one browser session: the token that was issued for it, when it started and when it ends.
// A session carries the principal it was opened with, so a request that presents the session id gets
// the same scopes the token had.
type Session struct {
	ID        string
	Principal Principal
	Started   time.Time
	Expires   time.Time
}

// Expired reports whether the session is past its lifetime at now.
func (s Session) Expired(now time.Time) bool { return !now.UTC().Before(s.Expires) }

// Sessions is the in-memory session table. It is bounded: a table that never forgets a session is a
// table that grows until the process stops, so Open refuses a session beyond the cap.
type Sessions struct {
	mu    sync.Mutex
	now   clock.Clock
	limit int
	byID  map[string]Session
}

// NewSessions returns a session table holding at most limit sessions.
func NewSessions(limit int, now clock.Clock) *Sessions {
	if limit <= 0 {
		limit = 64
	}
	return &Sessions{now: now, limit: limit, byID: map[string]Session{}}
}

// Open records a session for a verified principal and returns its id.
func (s *Sessions) Open(p Principal) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	if len(s.byID) >= s.limit {
		return Session{}, fmt.Errorf("auth: %d sessions are already open", s.limit)
	}
	session := Session{ID: idgen.New(idgen.Token), Principal: p, Started: clock.Now(s.now), Expires: p.Expires}
	s.byID[session.ID] = session
	return session, nil
}

// Lookup returns the session with the given id. An expired session is removed and reported missing,
// so a caller cannot keep using one past its lifetime.
func (s *Sessions) Lookup(id string) (Session, bool) {
	if id == "" {
		return Session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok {
		return Session{}, false
	}
	if session.Expired(clock.Now(s.now)) {
		delete(s.byID, id)
		return Session{}, false
	}
	return session, true
}

// Close removes one session.
func (s *Sessions) Close(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

// Len is the number of open sessions.
func (s *Sessions) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

// sweepLocked removes every expired session. The caller holds the lock.
func (s *Sessions) sweepLocked() {
	now := clock.Now(s.now)
	for id, session := range s.byID {
		if session.Expired(now) {
			delete(s.byID, id)
		}
	}
}

// Bearer reads the token a request presented: the value after "Bearer " in the Authorization header.
// Any other scheme is refused rather than read.
func Bearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return "", false
	}
	token := header[len(prefix):]
	if token == "" {
		return "", false
	}
	return token, true
}
