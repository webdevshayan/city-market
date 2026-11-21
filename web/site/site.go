// Package site is the City Market public web app — a standalone net/http
// application on its own Go module, deployed on its own domain (see
// vercel.json and README.md). It serves the storefront: home with the FAQ +
// JSON-LD, the pretty-URL shop, legal pages, sitemap/robots, static assets,
// and the visitor cart as a signed cookie.
//
// It has no database and no business rules. The catalog is fetched from the
// City Market API (the server deployment) via GET /api/v1/products; every
// price shown renders from that data, so the server stays the single source
// of truth for money and availability.
package site

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/webdevshayan/city-market/web/internal/cart"
	"github.com/webdevshayan/city-market/web/internal/csrf"
	"github.com/webdevshayan/city-market/web/internal/ratelimit"
)

//go:embed templates static
var content embed.FS

// Options configures the site. Empty fields get safe local defaults; see
// web/README.md for the deployment variables.
type Options struct {
	BaseURL    string // WEB_BASE_URL — canonical + sitemap base; https enables Secure cookies
	APIBaseURL string // API_BASE_URL — the City Market API (server deployment); empty = empty shop until configured
	CartSecret string // CART_SECRET — HMAC key for the cart cookie; empty = ephemeral per-process key
}

// Site is the assembled web application.
type Site struct {
	base     string
	secure   bool
	cat      CatalogSource
	basket   *cart.Cart
	protect  csrf.Protector
	mux      *http.ServeMux
	loggedDF bool // logged the catalog degradation once?
}

// New assembles the application: security headers, CSRF double-submit,
// per-IP rate limiting on the cart write, static assets, pages.
func New(opts Options) *Site {
	base := opts.BaseURL
	if base == "" {
		base = "http://localhost:3000"
	}
	secure := strings.HasPrefix(base, "https://")
	s := &Site{
		base:    base,
		secure:  secure,
		protect: csrf.Protector{Secure: secure},
		basket:  cart.New(opts.CartSecret, secure),
		mux:     http.NewServeMux(),
	}
	if opts.APIBaseURL != "" {
		s.cat = NewAPISource(opts.APIBaseURL)
	} else {
		s.cat = EmptySource{}
	}
	s.routes()
	return s
}

// ServeHTTP applies the middleware chain around the route table.
func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secureHeaders(s.protect.Middleware(s.mux)).ServeHTTP(w, r)
}

func (s *Site) routes() {
	staticRoot, err := fs.Sub(content, "static")
	if err != nil {
		log.Fatalf("static assets unavailable: %v", err)
	}

	mux := s.mux
	mux.Handle("GET /static/", http.StripPrefix("/static/", noDirectoryListing(http.FileServer(http.FS(staticRoot)))))
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /privacy", s.handlePrivacy)
	mux.HandleFunc("GET /terms", s.handleTerms)
	mux.HandleFunc("GET /shop", s.handleShop)
	mux.HandleFunc("GET /shop/{category}", s.handleShop)
	mux.HandleFunc("GET /shop/{category}/{brand}", s.handleShop)
	mux.HandleFunc("GET /shop/{category}/{brand}/{size}", s.handleShop)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.Handle("POST /cart/add", ratelimit.New(2, 8).Middleware(http.HandlerFunc(s.handleCartAdd)))
	// Catch-all: anything unmatched gets the branded 404. Go's mux routes the
	// most specific pattern first, so this never shadows the routes above.
	mux.HandleFunc("GET /", s.render404)
}

// withCatalog swaps the catalog source (test hook — the page tests run
// against a fixture source instead of a live API).
func (s *Site) withCatalog(src CatalogSource) *Site {
	s.cat = src
	return s
}

// catalog returns the current product snapshot, degrading gracefully: the
// last good API snapshot, then an empty grid — the storefront never
// hard-fails because a dependency blipped, and the degradation is logged.
func (s *Site) catalog(ctx context.Context) catalogView {
	products, err := s.cat.All(ctx)
	if err != nil && !s.loggedDF {
		s.loggedDF = true
		log.Printf("catalog source degraded, serving an empty grid: %v", err)
	}
	return catalogView{products: products}
}

// mustTpl parses a page template set: base.html defines the layout ("base"),
// each page file overrides the layout's "content" block.
func mustTpl(files ...string) *template.Template {
	return template.Must(template.ParseFS(content, files...))
}

// pageTpl builds a full-page template set: every page shares the base layout
// and the cart-count fragment rendered in its header.
func pageTpl(files ...string) *template.Template {
	return mustTpl(append([]string{"templates/base.html", "templates/partials/cart-count.html"}, files...)...)
}

var (
	tplHome      = pageTpl("templates/home.html")
	tplPrivacy   = pageTpl("templates/privacy.html")
	tplTerms     = pageTpl("templates/terms.html")
	tplShop      = pageTpl("templates/shop.html", "templates/partials/shop-main.html", "templates/partials/grid.html")
	tplShopMain  = mustTpl("templates/partials/shop-main.html", "templates/partials/grid.html")
	tplCartCount = mustTpl("templates/partials/cart-count.html")
	tplNotFound  = pageTpl("templates/notfound.html")
)

// PageData carries everything a page's template may need; each handler sets
// only the fields relevant to it.
// FilterLink is one filter chip; Active highlights the current selection.
type FilterLink struct {
	Name   string
	URL    string
	Active bool
}

type PageData struct {
	Title         string
	Description   string
	Path          string // request path; used for canonical + og:url
	BaseURL       string
	Year          int
	Faqs          []FaqItem   // home page FAQ section
	OrgJSONLD     template.JS // Organization node, emitted on every page
	JSONLD        template.JS // page-specific node (FAQPage, ItemList, …)
	Products      []Product
	CategoryLabel string // display labels of the active filters ("" = none)
	BrandLabel    string
	SizeLabel     string
	CatLinks      []FilterLink
	BrandLinks    []FilterLink
	SizeLinks     []FilterLink
	CSRFToken     string // embedded into hx-headers for state-changing HTMX
	CartCount     int    // items in the visitor's cart (header badge)
}

// baseData fills the fields every page shares.
func (s *Site) baseData(r *http.Request, title, description string) PageData {
	return PageData{
		Title:       title,
		Description: description,
		Path:        r.URL.Path,
		BaseURL:     s.base,
		Year:        time.Now().Year(),
		OrgJSONLD:   orgJSONLD(s.base),
		CSRFToken:   s.protect.FromRequest(r),
		CartCount:   s.basket.Count(r),
	}
}

// FaqItem feeds both the visible FAQ section and the FAQPage JSON-LD.
type FaqItem struct {
	Question string
	Answer   string
}

var faqs = []FaqItem{
	{Question: "Is the meat Halal?", Answer: "Yes — every shop is certified Halal before it can list, and certifications are re-checked by our admin team. If certification cannot be evidenced for an item, it is refunded in full."},
	{Question: "What commission do shops pay?", Answer: "A tiered monthly fee on app sales: 0% up to 50 orders, 5% up to 200, 7% up to 500, and 9% beyond that — capped at 7000 per month. Never interest."},
	{Question: "How much do riders earn?", Answer: "A fixed 60 per delivery plus 15 per kilometre, settled weekly."},
	{Question: "What does delivery cost me?", Answer: "One flat fee, shown clearly at checkout — no surge pricing and no hidden charges."},
	{Question: "What if an order goes wrong?", Answer: "Non-delivery is refunded 100%, wrong or spoiled items are refunded in full, and Halal doubts are refunded first and investigated after — see the refund policy in our Terms."},
}

// orgJSONLD renders the Organization graph node injected on every page.
func orgJSONLD(base string) template.JS {
	b, _ := json.Marshal(map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Organization",
		"name":        "City Market",
		"url":         base,
		"slogan":      "100% Halal delivery marketplace",
		"description": "Certified-Halal shops with tiered Ujrah commissions, fixed rider pay and one flat delivery fee.",
	})
	return template.JS(b)
}

// faqJSONLD renders a FAQPage graph node for answer-engine optimization.
func faqJSONLD(items []FaqItem) template.JS {
	qa := make([]map[string]any, len(items))
	for i, it := range items {
		qa[i] = map[string]any{
			"@type":          "Question",
			"name":           it.Question,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": it.Answer},
		}
	}
	b, _ := json.Marshal(map[string]any{
		"@context":   "https://schema.org",
		"@type":      "FAQPage",
		"mainEntity": qa,
	})
	return template.JS(b)
}

// renderHTML sends the named template of a page set as an HTML response.
// Page sets always name their layout "base"; HTMX partials use their own
// define names. Headers go out before the template runs, so a mid-render
// failure can only log — never corrupt the status line.
func renderHTML(w http.ResponseWriter, status int, tpl *template.Template, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", data.Path, err)
	}
}

// secureHeaders is the security baseline (Phase 1 review groundwork):
// every response carries a restrictive CSP with no inline script allowance,
// plus the standard no-sniff / frame / referrer guards.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}

// noDirectoryListing blocks index rendering for directory paths so only real
// files under /static/ are ever served (review finding M2).
func noDirectoryListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Site) handleHome(w http.ResponseWriter, r *http.Request) {
	data := s.baseData(r,
		"City Market — 100% Halal delivery marketplace",
		"Certified-Halal shops, fixed rider pay and transparent tiered commissions (Ujrah). One flat delivery fee at checkout.")
	data.Faqs = faqs
	data.JSONLD = faqJSONLD(faqs)
	renderHTML(w, http.StatusOK, tplHome, "base", data)
}

func (s *Site) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, http.StatusOK, tplPrivacy, "base", s.baseData(r,
		"Privacy Policy — City Market",
		"What City Market collects, why, who it is shared with, and your rights over your data."))
}

func (s *Site) handleTerms(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, http.StatusOK, tplTerms, "base", s.baseData(r,
		"Terms of Service — City Market",
		"The rules of the City Market marketplace, including the Halal commitment and the refund policy."))
}

func (s *Site) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"city-market-web"}`))
}

func (s *Site) render404(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, http.StatusNotFound, tplNotFound, "base", s.baseData(r,
		"Page not found — City Market",
		"That page does not exist or has moved. Browse the Halal market instead."))
}

// handleCartAdd adds a product to the visitor's cart (the signed cookie).
// CSRF is validated by the middleware before we get here — X-CSRF-Token
// from htmx's hx-headers, or the _csrf form field as the no-JS fallback.
func (s *Site) handleCartAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PostFormValue("product_id")
	if id == "" {
		http.Error(w, "missing product_id", http.StatusBadRequest)
		return
	}
	if _, ok := s.catalog(r.Context()).byID(id); !ok {
		http.Error(w, "unknown product", http.StatusBadRequest)
		return
	}
	count, ok := s.basket.Add(w, r, id)
	if !ok {
		http.Error(w, "cart capacity reached", http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		renderHTML(w, http.StatusOK, tplCartCount, "cartCount", PageData{CartCount: count})
		return
	}
	// No-JS fallback: back to the shop, badge refreshed on the next render.
	http.Redirect(w, r, "/shop", http.StatusSeeOther)
}
