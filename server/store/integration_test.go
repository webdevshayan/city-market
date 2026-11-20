//go:build integration

// Live verification against a real Turso database. Not run in CI (no
// DATABASE_URL secret there). Run locally with:
//
//	TEST_DATABASE_URL="$(grep '^DATABASE_URL=' .env | cut -d= -f2-)" \
//	  go test -tags integration ./store -v
//
// The DSN is never printed; failures report the failing stage only.
package store

import (
	"os"
	"testing"
)

func TestLiveTursoRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	s, err := Open(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	if err := s.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := s.SeedIfEmpty(SeedShops(), SeedRiders(), SeedProducts()); err != nil {
		t.Fatalf("seed-if-empty: %v", err)
	}
	if err := s.SeedIfEmpty(SeedShops(), SeedRiders(), SeedProducts()); err != nil {
		t.Fatalf("seed-if-empty twice must stay a no-op: %v", err)
	}

	products, err := s.Products()
	if err != nil {
		t.Fatalf("products: %v", err)
	}
	if len(products) < 8 {
		t.Fatalf("live products = %d, want >= 8 after seed-if-empty", len(products))
	}
	shops, err := s.Shops()
	if err != nil {
		t.Fatalf("shops: %v", err)
	}
	if len(shops) < 3 {
		t.Fatalf("live shops = %d, want >= 3", len(shops))
	}
	riders, err := s.Riders()
	if err != nil {
		t.Fatalf("riders: %v", err)
	}
	if len(riders) < 4 {
		t.Fatalf("live riders = %d, want >= 4", len(riders))
	}
	for _, p := range products {
		if p.ShopID == "" {
			t.Fatalf("live product %s has no shop id", p.ID)
		}
	}
}
