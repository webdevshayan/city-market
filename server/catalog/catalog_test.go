package catalog

import "testing"

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
			if got := len(Filter(tc.category, tc.brand, tc.size)); got != tc.want {
				t.Fatalf("Filter(%q,%q,%q) = %d products, want %d", tc.category, tc.brand, tc.size, got, tc.want)
			}
		})
	}
}

func TestValidFilter(t *testing.T) {
	if !ValidFilter("", "", "") {
		t.Fatal("empty filter must be valid")
	}
	if !ValidFilter("meat", "al-safwa", "1kg") {
		t.Fatal("full known triple must be valid")
	}
	if !ValidFilter("meat", "noor", "") {
		t.Fatal("known vocab with zero products is still a valid filter")
	}
	if ValidFilter("electronics", "", "") {
		t.Fatal("unknown category must be invalid")
	}
	if ValidFilter("meat", "madina", "2kg") {
		t.Fatal("unknown size must be invalid")
	}
}

func TestByIDAndPriceText(t *testing.T) {
	p, ok := ByID("beef-1kg")
	if !ok {
		t.Fatal("seed product must resolve by ID")
	}
	if p.PriceText() != "1400.00" {
		t.Fatalf("price text = %q, want 1400.00", p.PriceText())
	}
	if _, ok := ByID("nope"); ok {
		t.Fatal("unknown id must not resolve")
	}
}
