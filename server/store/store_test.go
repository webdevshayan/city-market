package store

import (
	"errors"
	"testing"

	// Registers the "sqlite" driver so the libsql driver accepts local
	// file::memory: DSNs. Test-only: the production binary never links it.
	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return s
}

func TestDisabledStoreIsAStub(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	if s.Enabled() {
		t.Fatal("empty DSN must disable the store")
	}
	if s.DB() != nil {
		t.Fatal("disabled store must expose no handle")
	}
	if err := s.EnsureSchema(); err != nil {
		t.Fatal("EnsureSchema must no-op when disabled")
	}
	if err := s.SeedIfEmpty(SeedShops(), SeedRiders(), SeedProducts()); err != nil {
		t.Fatal("SeedIfEmpty must no-op when disabled")
	}
	if _, err := s.Products(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
}

func TestSeedIfEmptyIsIdempotent(t *testing.T) {
	s := testStore(t)
	if err := s.SeedIfEmpty(SeedShops(), SeedRiders(), SeedProducts()); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := s.SeedIfEmpty(SeedShops(), SeedRiders(), SeedProducts()); err != nil {
		t.Fatalf("second seed must be a no-op, not a duplicate: %v", err)
	}

	products, err := s.Products()
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 8 {
		t.Fatalf("products = %d, want 8", len(products))
	}
	for _, p := range products {
		if p.ShopID == "" {
			t.Fatalf("product %s must carry a shop id", p.ID)
		}
	}

	shops, err := s.Shops()
	if err != nil {
		t.Fatal(err)
	}
	if len(shops) != 3 {
		t.Fatalf("shops = %d, want 3", len(shops))
	}

	riders, err := s.Riders()
	if err != nil {
		t.Fatal(err)
	}
	if len(riders) != 4 {
		t.Fatalf("riders = %d, want 4", len(riders))
	}
}

func TestDispatchFlagsRoundTrip(t *testing.T) {
	s := testStore(t)
	if err := s.SeedIfEmpty(SeedShops(), SeedRiders(), SeedProducts()); err != nil {
		t.Fatal(err)
	}

	shops, _ := s.Shops()
	for _, sh := range shops {
		if !sh.Active {
			t.Fatalf("all seed shops are active; %q is not", sh.ID)
		}
	}

	inactive := 0
	riders, _ := s.Riders()
	for _, rd := range riders {
		if !rd.Active {
			inactive++
		}
	}
	if inactive != 1 {
		t.Fatalf("want exactly one off-duty rider fixture, got %d", inactive)
	}
}
