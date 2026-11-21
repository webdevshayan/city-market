// Package catalog supplies the product listing behind the API. The seed
// implementation is a clearly-marked fixture set so development and tests
// run with zero configuration; it implements the same surface the
// Turso-backed provider uses.
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

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

// Provider supplies the catalog. The seed provider backs development and the
// no-database fallback; the DB provider snapshots Turso at boot. The shop
// UI, sitemap and cart validation all go through this interface, so nothing
// downstream cares which one is wired.
type Provider interface {
	All() []Product
	ByID(id string) (Product, bool)
	Filter(category, brand, size string) []Product
	Categories() []string
	Brands() []string
	Sizes() []string
	ValidFilter(category, brand, size string) bool
}

// NewSeed returns the provider backed by the launch fixtures.
func NewSeed() Provider { return &listProvider{products: seed} }

// NewDB returns a provider that snapshots the database catalog at boot.
// Live refresh without a restart arrives with the admin console (Phase 4).
func NewDB(source interface {
	Products() ([]Product, error)
}) (Provider, error) {
	products, err := source.Products()
	if err != nil {
		return nil, fmt.Errorf("catalog: load from database: %w", err)
	}
	if len(products) == 0 {
		return nil, errors.New("catalog: database catalog is empty")
	}
	return &listProvider{products: products}, nil
}

// listProvider serves a catalog from an in-memory product list; both the
// seed provider and the DB-backed snapshot use it.
type listProvider struct{ products []Product }

func (p *listProvider) All() []Product { return p.products }

func (p *listProvider) ByID(id string) (Product, bool) {
	for _, prod := range p.products {
		if prod.ID == id {
			return prod, true
		}
	}
	return Product{}, false
}

// Filter returns products matching every non-empty slug constraint.
func (p *listProvider) Filter(category, brand, size string) []Product {
	out := make([]Product, 0, len(p.products))
	for _, prod := range p.products {
		if category != "" && prod.Category != category {
			continue
		}
		if brand != "" && prod.Brand != brand {
			continue
		}
		if size != "" && prod.Size != size {
			continue
		}
		out = append(out, prod)
	}
	return out
}

// Categories, Brands and Sizes return the distinct vocabularies, in catalog
// order — these are the only slugs valid in a pretty URL segment.
func (p *listProvider) Categories() []string {
	return vocabOf(p.products, func(prod Product) string { return prod.Category })
}

func (p *listProvider) Brands() []string {
	return vocabOf(p.products, func(prod Product) string { return prod.Brand })
}

func (p *listProvider) Sizes() []string {
	return vocabOf(p.products, func(prod Product) string { return prod.Size })
}

// ValidFilter reports whether the slug triple uses only known vocabulary
// (empty segment = unconstrained). Unknown segments 404 so search engines
// never index filter combinations that cannot exist.
func (p *listProvider) ValidFilter(category, brand, size string) bool {
	return contains(p.Categories(), category) && contains(p.Brands(), brand) && contains(p.Sizes(), size)
}

func vocabOf(products []Product, key func(Product) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, prod := range products {
		v := key(prod)
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
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
