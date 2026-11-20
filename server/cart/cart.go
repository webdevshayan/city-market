// Package cart keeps the per-session cart behind a small interface. The
// in-memory implementation is bounded (it backed the Phase 2 demo); the
// Turso-backed implementation added in Phase 3 persists carts in the
// cart_items table created by the store package schema. main() wires one of
// the two depending on DATABASE_URL — callers never care which is active.
package cart

import (
	"database/sql"
	"sync"
)

// Store is the cart backend interface.
type Store interface {
	// Add increments the quantity of productID in the session's cart and
	// returns the session's new total item count. ok=false signals a rejected
	// write (capacity reached, or a database error on the DB backend).
	Add(sessionID, productID string) (count int64, ok bool)
	// Count returns the total item count for a session (0 for unknown
	// sessions).
	Count(sessionID string) int64
}

const maxSessions = 50_000

// Memory is the bounded in-memory cart store.
type Memory struct {
	mu       sync.Mutex
	sessions map[string]map[string]int64
}

// NewMemory returns an empty in-memory cart store.
func NewMemory() *Memory {
	return &Memory{sessions: make(map[string]map[string]int64)}
}

// Add increments the quantity of productID in the session's cart. ok=false
// when the store is full — new sessions are rejected rather than evicting
// live carts.
func (s *Memory) Add(sessionID, productID string) (count int64, ok bool) {
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
func (s *Memory) Count(sessionID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, q := range s.sessions[sessionID] {
		total += q
	}
	return total
}

// DBStore persists carts in Turso via the cart_items table (schema owned by
// the store package). Quantity updates are single atomic SQL statements.
type DBStore struct {
	db *sql.DB
}

// NewDB returns a Turso-backed cart store. The handle must be non-nil —
// callers check store.Enabled() before wiring.
func NewDB(db *sql.DB) *DBStore {
	return &DBStore{db: db}
}

// Add increments the quantity atomically (INSERT … ON CONFLICT DO UPDATE)
// and returns the session's refreshed total. ok=false on database errors —
// the caller answers 5xx rather than silently dropping the item.
func (s *DBStore) Add(sessionID, productID string) (int64, bool) {
	if _, err := s.db.Exec(
		`INSERT INTO cart_items (session_id, product_id, qty) VALUES (?, ?, 1)
		 ON CONFLICT (session_id, product_id) DO UPDATE SET qty = qty + 1`,
		sessionID, productID); err != nil {
		return 0, false
	}
	return s.Count(sessionID), true
}

// Count returns the total item count for a session (0 for unknown sessions
// or read errors — a broken badge must never block shopping).
func (s *DBStore) Count(sessionID string) int64 {
	var total int64
	if err := s.db.QueryRow(
		`SELECT COALESCE(SUM(qty), 0) FROM cart_items WHERE session_id = ?`,
		sessionID).Scan(&total); err != nil {
		return 0
	}
	return total
}
