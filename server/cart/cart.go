// Package cart keeps the per-session cart for the Phase 2 demo checkout
// flow. It is deliberately in-memory and bounded: on Vercel, instances are
// ephemeral, and Turso-backed persistence replaces this when the database
// phase lands — do not treat this as production cart storage.
package cart

import "sync"

const maxSessions = 50_000

// Store maps a session ID to its product quantities.
type Store struct {
	mu       sync.Mutex
	sessions map[string]map[string]int64
}

// NewStore returns an empty cart store.
func NewStore() *Store {
	return &Store{sessions: make(map[string]map[string]int64)}
}

// Add increments the quantity of productID in the session's cart and returns
// the session's new total item count. ok=false when the store is full — new
// sessions are rejected rather than evicting live carts.
func (s *Store) Add(sessionID, productID string) (count int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, exists := s.sessions[sessionID]
	if !exists {
		if len(s.sessions) >= maxSessions {
			return 0, false
		}
		items = make(map[string]int64)
		s.sessions[sessionID] = items
	}
	items[productID]++
	var total int64
	for _, q := range items {
		total += q
	}
	return total, true
}

// Count returns the total item count for a session (0 for unknown sessions).
func (s *Store) Count(sessionID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, q := range s.sessions[sessionID] {
		total += q
	}
	return total
}
