// Package catalog supplies the product listing behind the shop. The seed
// implementation is a clearly-marked fixture set so the pretty-URL shop UI
// can be built and tested before the Turso-backed provider lands with the
// database phase; it implements the same surface the DB provider will use.
package catalog

import (
	"fmt"
	"strings"
)

// CurrencyCode is the ISO code used in structured-data offers. The product
// spec does not name a currency; PKR is the launch-market assumption and this
// is the single place to change it.
const CurrencyCode = "PKR"

// Product is one listing. Category/Brand/Size hold URL slugs (not display
// names) so shareable pretty URLs map 1:1 onto filters; ShopID ties the
// listing to the shop that fulfils it (used by the matching engine).
type Product struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Brand      string `json:"brand"`
	Size       string `json:"size"`
	PriceMinor int64  `json:"price_minor"`
	Image      string `json:"image"` // path under /static/img/
	ShopID     string `json:"shop_id"`
}

// PriceText renders the price as "major.minor" for display.
func (p Product) PriceText() string {
	return fmt.Sprintf("%d.%02d", p.PriceMinor/100, p.PriceMinor%100)
}

// seed is the fixture catalog until Turso backs it. ShopID values reference
// the launch shops in the store package's seed fixtures.
var seed = []Product{
	{ID: "beef-1kg", Name: "Halal Beef Boneless", Category: "meat", Brand: "al-safwa", Size: "1kg", PriceMinor: 140000, Image: "/static/img/meat.svg", ShopID: "s-central"},
	{ID: "chicken-1kg", Name: "Whole Chicken, cleaned", Category: "meat", Brand: "madina", Size: "1kg", PriceMinor: 85000, Image: "/static/img/meat.svg", ShopID: "s-central"},
	{ID: "mutton-1kg", Name: "Mutton Leg Cuts", Category: "meat", Brand: "madina", Size: "1kg", PriceMinor: 240000, Image: "/static/img/meat.svg", ShopID: "s-north"},
	{ID: "rice-5kg", Name: "Basmati Rice", Category: "grocery", Brand: "noor", Size: "5kg", PriceMinor: 220000, Image: "/static/img/grocery.svg", ShopID: "s-north"},
	{ID: "atta-10kg", Name: "Chakki Atta", Category: "grocery", Brand: "noor", Size: "10kg", PriceMinor: 120000, Image: "/static/img/grocery.svg", ShopID: "s-south"},
	{ID: "dates-500g", Name: "Ajwa Dates", Category: "grocery", Brand: "barakah", Size: "500g", PriceMinor: 95000, Image: "/static/img/grocery.svg", ShopID: "s-south"},
	{ID: "naan-10pcs", Name: "Frozen Naan", Category: "bakery", Brand: "barakah", Size: "10pcs", PriceMinor: 35000, Image: "/static/img/bakery.svg", ShopID: "s-central"},
	{ID: "samosa-12pcs", Name: "Chicken Samosa Pack", Category: "bakery", Brand: "barakah", Size: "12pcs", PriceMinor: 42000, Image: "/static/img/bakery.svg", ShopID: "s-central"},
}

// Seed returns a copy of the launch fixture products — the single source of
// truth for both the in-memory provider and the database seeding path.
func Seed() []Product {
	out := make([]Product, len(seed))
	copy(out, seed)
	return out
}

// Filter returns products matching every non-empty slug constraint.
func Filter(category, brand, size string) []Product {
	out := make([]Product, 0, len(seed))
	for _, p := range seed {
		if category != "" && p.Category != category {
			continue
		}
		if brand != "" && p.Brand != brand {
			continue
		}
		if size != "" && p.Size != size {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ByID looks a product up by its ID; ok=false when unknown.
func ByID(id string) (Product, bool) {
	for _, p := range seed {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

// Categories, Brands and Sizes return the distinct vocabularies, in catalog
// order — these are the only slugs valid in a pretty URL segment.
func Categories() []string { return vocabOf(func(p Product) string { return p.Category }) }
func Brands() []string     { return vocabOf(func(p Product) string { return p.Brand }) }
func Sizes() []string      { return vocabOf(func(p Product) string { return p.Size }) }

func vocabOf(key func(Product) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range seed {
		v := key(p)
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// ValidFilter reports whether the slug triple uses only known vocabulary
// (empty segment = unconstrained). Unknown segments 404 so search engines
// never index filter combinations that cannot exist.
func ValidFilter(category, brand, size string) bool {
	return contains(Categories(), category) && contains(Brands(), brand) && contains(Sizes(), size)
}

func contains(list []string, slug string) bool {
	if slug == "" {
		return true
	}
	for _, v := range list {
		if v == slug {
			return true
		}
	}
	return false
}

// Display turns a slug into a readable label ("al-safwa" → "Al Safwa").
func Display(slug string) string {
	if slug == "" {
		return ""
	}
	words := strings.Split(strings.ReplaceAll(slug, "-", " "), " ")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
