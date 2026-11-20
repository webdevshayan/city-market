package cart

import (
	"database/sql"
	"testing"

	// Registers the "libsql" driver (same one production uses) plus a pure-Go
	// sqlite driver so local file::memory: DSNs work. Both test-only: the
	// production binary links libsql through the store package and never
	// links modernc.
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

func TestMemoryCounts(t *testing.T) {
	var s Store = NewMemory()
	if n, ok := s.Add("s1", "a"); !ok || n != 1 {
		t.Fatalf("first add = %d,%v; want 1,true", n, ok)
	}
	if n, _ := s.Add("s1", "a"); n != 2 {
		t.Fatalf("repeat add of same product = %d, want 2", n)
	}
	if n, _ := s.Add("s1", "b"); n != 3 {
		t.Fatalf("add of second product = %d, want 3", n)
	}
	if s.Count("s1") != 3 {
		t.Fatalf("Count(s1) = %d, want 3", s.Count("s1"))
	}
	if s.Count("unknown") != 0 {
		t.Fatal("unknown session must count 0")
	}
}

// The DB backend must behave exactly like the memory backend — both satisfy
// the same interface contract.
func TestDBCounts(t *testing.T) {
	db, err := sql.Open("libsql", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS cart_items (
		session_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		qty INTEGER NOT NULL,
		PRIMARY KEY (session_id, product_id)
	)`); err != nil {
		t.Fatalf("schema: %v", err)
	}

	var s Store = NewDB(db)
	if n, ok := s.Add("sess1", "beef-1kg"); !ok || n != 1 {
		t.Fatalf("db first add = %d,%v; want 1,true", n, ok)
	}
	if n, _ := s.Add("sess1", "beef-1kg"); n != 2 {
		t.Fatalf("db repeat add = %d, want 2 (ON CONFLICT must increment)", n)
	}
	if n, _ := s.Add("sess1", "rice-5kg"); n != 3 {
		t.Fatalf("db second product = %d, want 3", n)
	}
	if s.Count("sess1") != 3 {
		t.Fatalf("db Count(sess1) = %d, want 3", s.Count("sess1"))
	}
	if s.Count("other-session") != 0 {
		t.Fatal("db sessions must be isolated")
	}
}
