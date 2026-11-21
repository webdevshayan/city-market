package catalog

import (
	"errors"
	"testing"
)

var seedCat = NewSeed()

func TestFilterHierarchies(t *testing.T) {
	tests := []struct {
		name                  string
		category, brand, size string
		want                  int
	}{
		{"no filters returns everything", "", "", "", 8},
		{"category only", "meat", "", "", 3},
		{"category + brand", "meat", "madina", "", 2},
		{"full triple", "meat", "madina", "1kg", 2},
		{"size only within category", "bakery", "", "12pcs", 1},
		{"valid but empty combination", "grocery", "al-safwa", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(seedCat.Filter(tc.category, tc.brand, tc.size)); got != tc.want {
				t.Fatalf("Filter(%q,%q,%q) = %d products, want %d", tc.category, tc.brand, tc.size, got, tc.want)
			}
		})
	}
}

func TestValidFilter(t *testing.T) {
	if !seedCat.ValidFilter("", "", "") {
		t.Fatal("empty filter must be valid")
	}
	if !seedCat.ValidFilter("meat", "al-safwa", "1kg") {
		t.Fatal("full known triple must be valid")
	}
	if !seedCat.ValidFilter("meat", "noor", "") {
		t.Fatal("known vocab with zero products is still a valid filter")
	}
	if seedCat.ValidFilter("electronics", "", "") {
		t.Fatal("unknown category must be invalid")
	}
	if seedCat.ValidFilter("meat", "madina", "2kg") {
		t.Fatal("unknown size must be invalid")
	}
}

func TestByID(t *testing.T) {
	p, ok := seedCat.ByID("beef-1kg")
	if !ok {
		t.Fatal("seed product must resolve by ID")
	}
	if p.ShopID == "" {
		t.Fatal("seed products must carry a shop id for the matching engine")
	}
	if _, ok := seedCat.ByID("nope"); ok {
		t.Fatal("unknown id must not resolve")
	}
}

func TestSeedIsACopy(t *testing.T) {
	s := Seed()
	if len(s) != 8 {
		t.Fatalf("Seed() = %d products, want 8", len(s))
	}
	s[0].Name = "mutated"
	if Seed()[0].Name == "mutated" {
		t.Fatal("Seed() must return a copy, not the live slice")
	}
}

// fakeSource stands in for the store in provider-construction tests.
type fakeSource struct {
	products []Product
	err      error
}

func (f fakeSource) Products() ([]Product, error) { return f.products, f.err }

func TestNewDBProvider(t *testing.T) {
	p, err := NewDB(fakeSource{products: []Product{
		{ID: "db-1", Name: "DB Product", Category: "meat", Brand: "db-brand", Size: "1kg", PriceMinor: 10000, ShopID: "s-db"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(p.Filter("meat", "", "")); got != 1 {
		t.Fatalf("db provider filter = %d, want 1", got)
	}
	if _, ok := p.ByID("db-1"); !ok {
		t.Fatal("db provider must resolve its products")
	}
	if _, err := NewDB(fakeSource{}); err == nil {
		t.Fatal("an empty database catalog must fail loudly, not serve an empty shop")
	}
	if _, err := NewDB(fakeSource{err: errors.New("boom")}); err == nil {
		t.Fatal("source errors must propagate")
	}
}
