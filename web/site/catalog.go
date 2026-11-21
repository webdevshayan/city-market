// Catalog plumbing for the web app: the product model, the vocabulary and
// filter helpers behind the pretty shop URLs, and the CatalogSource that
// fetches products from the City Market API (the separate server
// deployment). The web app keeps NO catalog of its own — degraded modes
// serve the last good snapshot, never invented data.
package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CurrencyCode is the ISO code used in structured-data offers. The product
// spec does not name a currency; PKR is the launch-market assumption and
// this is the single place to change it.
const CurrencyCode = "PKR"

// Product mirrors one row of GET /api/v1/products (the API's JSON, snake_case).
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

// PriceText renders the price as "major.minor" for display — the same
// convention the templates have always rendered.
func (p Product) PriceText() string {
	return fmt.Sprintf("%d.%02d", p.PriceMinor/100, p.PriceMinor%100)
}

// CatalogSource supplies products for the shop pages. Implementations must
// be safe for concurrent use.
type CatalogSource interface {
	All(ctx context.Context) ([]Product, error)
}

// ErrCatalogUnavailable means no API snapshot is available at all (the API
// has never answered since this process started). Handlers degrade to an
// empty grid — the storefront stays up, honestly empty.
var ErrCatalogUnavailable = errors.New("catalog API unavailable and no snapshot cached")

// APISource fetches GET /api/v1/products from the City Market server and
// keeps the last good snapshot in memory: when the API blips, the
// storefront serves the stale catalog instead of failing. There is no seed
// fallback here on purpose — the web app must not carry a second copy of
// the catalog that can drift from the server's.
type APISource struct {
	base   string
	client *http.Client

	mu   sync.RWMutex
	last []Product
}

// NewAPISource builds the API-backed catalog source.
func NewAPISource(base string) *APISource {
	return &APISource{
		base:   strings.TrimRight(base, "/"),
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// All returns the current catalog, or the last good snapshot while the API
// is down, or ErrCatalogUnavailable if nothing has ever loaded.
func (s *APISource) All(ctx context.Context) ([]Product, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/api/v1/products", nil)
	if err != nil {
		return s.fallback()
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return s.fallback()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s.fallback()
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return s.fallback()
	}
	var products []Product
	if err := json.Unmarshal(body, &products); err != nil || len(products) == 0 {
		return s.fallback()
	}
	s.mu.Lock()
	s.last = products
	s.mu.Unlock()
	return products, nil
}

func (s *APISource) fallback() ([]Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.last) > 0 {
		return s.last, nil // stale but real data beats an empty shop
	}
	return nil, ErrCatalogUnavailable
}

// EmptySource is the zero-config source (no API_BASE_URL): the shop renders
// its empty state until an API is configured.
type EmptySource struct{}

// All always reports the catalog unavailable.
func (EmptySource) All(context.Context) ([]Product, error) {
	return nil, ErrCatalogUnavailable
}

// catalogView wraps a product snapshot with the vocabulary and filter
// helpers behind the pretty shop URLs (ported from the server's catalog
// provider semantics).
type catalogView struct{ products []Product }

func (c catalogView) byID(id string) (Product, bool) {
	for _, p := range c.products {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

// filter returns products matching every non-empty slug constraint.
func (c catalogView) filter(category, brand, size string) []Product {
	out := make([]Product, 0, len(c.products))
	for _, p := range c.products {
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

func (c catalogView) categories() []string {
	return vocabOf(c.products, func(p Product) string { return p.Category })
}
func (c catalogView) brands() []string {
	return vocabOf(c.products, func(p Product) string { return p.Brand })
}
func (c catalogView) sizes() []string {
	return vocabOf(c.products, func(p Product) string { return p.Size })
}

// validFilter reports whether the slug triple uses only known vocabulary
// (empty segment = unconstrained). Unknown segments 404 so search engines
// never index filter combinations that cannot exist.
func (c catalogView) validFilter(category, brand, size string) bool {
	return contains(c.categories(), category) && contains(c.brands(), brand) && contains(c.sizes(), size)
}

func vocabOf(products []Product, key func(Product) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range products {
		v := key(p)
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
