// Command server is City Market's HTTP server. It is a plain net/http server
// so it runs unchanged locally (go run ./server) and on Vercel's Go runtime,
// which runs a standard Go server from the project root and expects it to
// listen on the PORT environment variable.
//
// Phase 2 scope: semantic HTML layout with a global footer, legal pages,
// FAQ + JSON-LD, the HTMX shop with pretty shareable URLs, dynamic
// sitemap.xml/robots.txt, CSRF middleware for HTMX posts, and the strict CSP
// started in Phase 1.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/webdevshayan/city-market/server/cart"
	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/csrf"
	"github.com/webdevshayan/city-market/server/finance"
)

//go:embed templates static
var content embed.FS

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
	tplGrid      = mustTpl("templates/partials/grid.html")
	tplCartCount = mustTpl("templates/partials/cart-count.html")
	tplNotFound  = pageTpl("templates/notfound.html")
)

// carts holds the ephemeral Phase 2 demo carts; the Turso-backed store
// replaces it when the database phase lands.
var carts = cart.NewStore()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	csrf.SecureCookie = strings.HasPrefix(baseURL(), "https://")
	addr := ":" + port
	log.Printf("city-market server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, secureHeaders(csrf.Middleware(sessionCookie(newMux())))))
}

func newMux() *http.ServeMux {
	staticRoot, err := fs.Sub(content, "static")
	if err != nil {
		log.Fatalf("static assets unavailable: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", noDirectoryListing(http.FileServer(http.FS(staticRoot)))))
	mux.HandleFunc("GET /{$}", handleHome)
	mux.HandleFunc("GET /privacy", handlePrivacy)
	mux.HandleFunc("GET /terms", handleTerms)
	mux.HandleFunc("GET /shop", handleShop)
	mux.HandleFunc("GET /shop/{category}", handleShop)
	mux.HandleFunc("GET /shop/{category}/{brand}", handleShop)
	mux.HandleFunc("GET /shop/{category}/{brand}/{size}", handleShop)
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/finance/ujrah", handleUjrah)
	mux.HandleFunc("GET /api/v1/finance/rider", handleRiderPayout)
	mux.HandleFunc("GET /api/v1/finance/checkout-fee", handleCheckoutFee)
	mux.HandleFunc("GET /api/v1/finance/simulate-month", handleSimulateMonth)
	mux.HandleFunc("GET /sitemap.xml", handleSitemap)
	mux.HandleFunc("GET /robots.txt", handleRobots)
	mux.HandleFunc("POST /cart/add", handleCartAdd)
	// Catch-all: anything unmatched gets the branded 404. Go's mux routes the
	// most specific pattern first, so this never shadows the routes above.
	mux.HandleFunc("GET /", render404)
	return mux
}

func baseURL() string {
	if b := os.Getenv("BASE_URL"); b != "" {
		return b
	}
	return "http://localhost:3000"
}

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
	Products      []catalog.Product
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
func baseData(r *http.Request, title, description string) PageData {
	return PageData{
		Title:       title,
		Description: description,
		Path:        r.URL.Path,
		BaseURL:     baseURL(),
		Year:        time.Now().Year(),
		OrgJSONLD:   orgJSONLD(baseURL()),
		CSRFToken:   csrf.FromRequest(r),
		CartCount:   int(carts.Count(sessionID(r))),
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

func handleHome(w http.ResponseWriter, r *http.Request) {
	data := baseData(r,
		"City Market — 100% Halal delivery marketplace",
		"Certified-Halal shops, fixed rider pay and transparent tiered commissions (Ujrah). One flat delivery fee at checkout.")
	data.Faqs = faqs
	data.JSONLD = faqJSONLD(faqs)
	renderHTML(w, http.StatusOK, tplHome, "base", data)
}

// handleShop serves the product grid at pretty, shareable filter URLs:
// /shop, /shop/{category}, /shop/{category}/{brand}, /shop/{category}/{brand}/{size}.
// Unknown vocabulary 404s so engines never index impossible filter pages;
// valid-but-empty combinations render a friendly empty grid.
func handleShop(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	brand := r.PathValue("brand")
	size := r.PathValue("size")
	if !catalog.ValidFilter(category, brand, size) {
		if r.Header.Get("HX-Request") == "true" {
			// htmx does not swap 4xx responses by default, so the grid stays
			// as-is; direct loads of a hand-crafted bad URL still get the
			// full branded 404 page below.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		render404(w, r)
		return
	}
	data := baseData(r, shopTitle(category, brand, size),
		"Certified-Halal products with transparent prices. Product-first listings: image, name, price, add to cart.")
	data.CategoryLabel = catalog.Display(category)
	data.BrandLabel = catalog.Display(brand)
	data.SizeLabel = catalog.Display(size)
	data.Products = catalog.Filter(category, brand, size)
	data.JSONLD = itemListJSONLD(data.BaseURL, data.Products)

	catLinks := []FilterLink{{Name: "All", URL: "/shop", Active: category == ""}}
	for _, c := range catalog.Categories() {
		catLinks = append(catLinks, FilterLink{Name: catalog.Display(c), URL: "/shop/" + c, Active: c == category})
	}
	data.CatLinks = catLinks
	if category != "" {
		catBase := "/shop/" + category
		for _, b := range catalog.Brands() {
			data.BrandLinks = append(data.BrandLinks, FilterLink{Name: catalog.Display(b), URL: catBase + "/" + b, Active: b == brand})
		}
		if brand != "" {
			brandBase := catBase + "/" + brand
			for _, s := range catalog.Sizes() {
				data.SizeLinks = append(data.SizeLinks, FilterLink{Name: catalog.Display(s), URL: brandBase + "/" + s, Active: s == size})
			}
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		// HTMX swaps just the shop-main region; hx-push-url keeps the pretty
		// URL in the address bar, and direct loads of that URL render the
		// full page with the same pre-filtered state.
		renderHTML(w, http.StatusOK, tplShopMain, "shopMain", data)
		return
	}
	renderHTML(w, http.StatusOK, tplShop, "base", data)
}

func shopTitle(category, brand, size string) string {
	var parts []string
	for _, d := range []string{catalog.Display(category), catalog.Display(brand), catalog.Display(size)} {
		if d != "" {
			parts = append(parts, d)
		}
	}
	if len(parts) == 0 {
		return "Shop — City Market"
	}
	return strings.Join(parts, " · ") + " — City Market"
}

// itemListJSONLD renders the ItemList of Products (price + availability) for
// answer engines on shop pages.
func itemListJSONLD(base string, products []catalog.Product) template.JS {
	elems := make([]map[string]any, len(products))
	for i, p := range products {
		elems[i] = map[string]any{
			"@type":    "ListItem",
			"position": i + 1,
			"item": map[string]any{
				"@type": "Product",
				"name":  p.Name,
				"image": base + p.Image,
				"offers": map[string]any{
					"@type":         "Offer",
					"price":         p.PriceText(),
					"priceCurrency": catalog.CurrencyCode,
					"availability":  "https://schema.org/InStock",
				},
			},
		}
	}
	b, _ := json.Marshal(map[string]any{
		"@context":        "https://schema.org",
		"@type":           "ItemList",
		"name":            "City Market products",
		"itemListElement": elems,
	})
	return template.JS(b)
}

func render404(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, http.StatusNotFound, tplNotFound, "base", baseData(r,
		"Page not found — City Market",
		"That page does not exist or has moved. Browse the Halal market instead."))
}

// sessionID returns the visitor's cart session id from its cookie ("" if none).
func sessionID(r *http.Request) string {
	c, err := r.Cookie("cm_sid")
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

// randomHex returns n CSPRNG bytes hex-encoded, for session cookies.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("entropy unavailable: " + err.Error()) // CSPRNG failure is unrecoverable
	}
	return hex.EncodeToString(b)
}

// secureCookies marks session cookies Secure when we serve https (the CSRF
// cookie gets the same treatment via csrf.SecureCookie, set in main).
var secureCookies = strings.HasPrefix(baseURL(), "https://")

// sessionCookie ensures every visitor carries a cart session id cookie.
func sessionCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			if sessionID(r) == "" {
				http.SetCookie(w, &http.Cookie{
					Name:     "cm_sid",
					Value:    randomHex(16),
					Path:     "/",
					MaxAge:   86400 * 30,
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
					Secure:   secureCookies,
				})
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleCartAdd adds a product to the visitor's cart. CSRF is validated by
// the middleware before we get here — X-CSRF-Token from htmx's hx-headers,
// or the _csrf form field as the no-JS fallback.
func handleCartAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PostFormValue("product_id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing product_id")
		return
	}
	if _, ok := catalog.ByID(id); !ok {
		writeErr(w, http.StatusBadRequest, "unknown product")
		return
	}
	sid := sessionID(r)
	if sid == "" {
		sid = randomHex(16)
		http.SetCookie(w, &http.Cookie{
			Name:     "cm_sid",
			Value:    sid,
			Path:     "/",
			MaxAge:   86400 * 30,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   secureCookies,
		})
	}
	count, ok := carts.Add(sid, id)
	if !ok {
		writeErr(w, http.StatusServiceUnavailable, "cart capacity reached")
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		renderHTML(w, http.StatusOK, tplCartCount, "cartCount", PageData{CartCount: int(count)})
		return
	}
	// No-JS fallback: back to the shop, badge refreshed on the next render.
	http.Redirect(w, r, "/shop", http.StatusSeeOther)
}

// urlset/urlEntry are the sitemap protocol structures.
type urlEntry struct {
	Loc string `xml:"loc"`
}

type urlset struct {
	XMLName xml.Name   `xml:"urlset"`
	Ns      string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

// sitemapPaths lists every indexable path: static pages, the shop, and every
// filter combination that actually has products — so engines never crawl
// empty filter pages.
func sitemapPaths() []string {
	paths := []string{"/", "/privacy", "/terms", "/shop"}
	for _, c := range catalog.Categories() {
		catPath := "/shop/" + c
		paths = append(paths, catPath)
		for _, b := range catalog.Brands() {
			if len(catalog.Filter(c, b, "")) == 0 {
				continue
			}
			brandPath := catPath + "/" + b
			paths = append(paths, brandPath)
			for _, s := range catalog.Sizes() {
				if len(catalog.Filter(c, b, s)) > 0 {
					paths = append(paths, brandPath+"/"+s)
				}
			}
		}
	}
	return paths
}

// handleSitemap generates /sitemap.xml from the catalog so every product
// filter page is discoverable; the Turso-backed provider feeds the same
// builder once the database phase lands.
func handleSitemap(w http.ResponseWriter, r *http.Request) {
	urls := make([]urlEntry, 0, 16)
	for _, p := range sitemapPaths() {
		urls = append(urls, urlEntry{Loc: baseURL() + p})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if _, err := io.WriteString(w, xml.Header); err == nil {
		if err := enc.Encode(urlset{
			Ns:   "http://www.sitemaps.org/schemas/sitemap/0.9",
			URLs: urls,
		}); err != nil {
			log.Printf("render sitemap: %v", err)
		}
	}
}

// handleRobots serves /robots.txt, pointing crawlers at the sitemap.
func handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /static/\n\nSitemap: %s/sitemap.xml\n", baseURL())
}

func handlePrivacy(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, http.StatusOK, tplPrivacy, "base", baseData(r,
		"Privacy Policy — City Market",
		"What City Market collects, why, who it is shared with, and your rights over your data."))
}

func handleTerms(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, http.StatusOK, tplTerms, "base", baseData(r,
		"Terms of Service — City Market",
		"The rules of the City Market marketplace, including the Halal commitment and the refund policy."))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "city-market-server"})
}

func handleUjrah(w http.ResponseWriter, r *http.Request) {
	orders, ok, err := queryInt64(r, "orders")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "orders must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: orders")
		return
	}
	salesMinor, ok, err := queryInt64(r, "sales_minor")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "sales_minor must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: sales_minor")
		return
	}

	commissionMinor, rateBps, capped, err := finance.CommissionMinor(orders, salesMinor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orders":            orders,
		"sales_minor":       salesMinor,
		"rate_bps":          rateBps,
		"rate_percent":      rateBps / 100,
		"commission_minor":  commissionMinor,
		"commission_major":  commissionMinor / finance.MinorPerUnit,
		"commission_capped": capped,
	})
}

func handleRiderPayout(w http.ResponseWriter, r *http.Request) {
	deliveries, ok, err := queryInt64(r, "deliveries")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "deliveries must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: deliveries")
		return
	}
	km, ok, err := queryInt64(r, "km")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "km must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: km")
		return
	}

	payoutMinor, baseMinor, distanceMinor, err := finance.RiderPayoutMinor(deliveries, km)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deliveries":     deliveries,
		"km":             km,
		"base_minor":     baseMinor,
		"distance_minor": distanceMinor,
		"payout_minor":   payoutMinor,
		"payout_major":   payoutMinor / finance.MinorPerUnit,
	})
}

func handleCheckoutFee(w http.ResponseWriter, r *http.Request) {
	feeMinor := finance.CustomerDeliveryFeeMinor()
	writeJSON(w, http.StatusOK, map[string]any{
		"fee_minor": feeMinor,
		"fee_major": feeMinor / finance.MinorPerUnit,
	})
}

func handleSimulateMonth(w http.ResponseWriter, r *http.Request) {
	// All parameters are optional and default to zero so the endpoint can be
	// probed easily; production callers pass the full month's activity.
	orders, _, err := queryInt64(r, "orders")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "orders must be an integer")
		return
	}
	salesMinor, _, err := queryInt64(r, "sales_minor")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "sales_minor must be an integer")
		return
	}
	deliveries, _, err := queryInt64(r, "deliveries")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "deliveries must be an integer")
		return
	}
	km, _, err := queryInt64(r, "km")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "km must be an integer")
		return
	}

	summary, err := finance.SimulateMonth(orders, salesMinor, deliveries, km)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func queryInt64(r *http.Request, key string) (int64, bool, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, true, err
	}
	return v, true, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
