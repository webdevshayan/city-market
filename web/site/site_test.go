package site

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// stubSource serves the fixture catalog the page tests were written
// against — the same rows the server's seed catalog carries. The web app
// itself ships no catalog; tests own their fixtures.
type stubSource struct{ products []Product }

func (s stubSource) All(context.Context) ([]Product, error) { return s.products, nil }

func testCatalog() stubSource {
	return stubSource{products: []Product{
		{ID: "beef-1kg", Name: "Halal Beef Boneless", Category: "meat", Brand: "al-safwa", Size: "1kg", PriceMinor: 140000, Image: "/static/img/meat.svg", ShopID: "s-central"},
		{ID: "chicken-1kg", Name: "Whole Chicken, cleaned", Category: "meat", Brand: "madina", Size: "1kg", PriceMinor: 85000, Image: "/static/img/meat.svg", ShopID: "s-central"},
		{ID: "mutton-1kg", Name: "Mutton Leg Cuts", Category: "meat", Brand: "madina", Size: "1kg", PriceMinor: 240000, Image: "/static/img/meat.svg", ShopID: "s-north"},
		{ID: "rice-5kg", Name: "Basmati Rice", Category: "grocery", Brand: "noor", Size: "5kg", PriceMinor: 220000, Image: "/static/img/grocery.svg", ShopID: "s-north"},
		{ID: "atta-10kg", Name: "Chakki Atta", Category: "grocery", Brand: "noor", Size: "10kg", PriceMinor: 120000, Image: "/static/img/grocery.svg", ShopID: "s-south"},
		{ID: "dates-500g", Name: "Ajwa Dates", Category: "grocery", Brand: "barakah", Size: "500g", PriceMinor: 95000, Image: "/static/img/grocery.svg", ShopID: "s-south"},
		{ID: "naan-10pcs", Name: "Frozen Naan", Category: "bakery", Brand: "barakah", Size: "10pcs", PriceMinor: 35000, Image: "/static/img/bakery.svg", ShopID: "s-central"},
		{ID: "samosa-12pcs", Name: "Chicken Samosa Pack", Category: "bakery", Brand: "barakah", Size: "12pcs", PriceMinor: 42000, Image: "/static/img/bakery.svg", ShopID: "s-central"},
	}}
}

// newTestSite assembles the app with the fixture catalog over plain http.
func newTestSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(Options{}).withCatalog(testCatalog()))
	t.Cleanup(srv.Close)
	return srv
}

func TestPagesRenderFullLayout(t *testing.T) {
	srv := newTestSite(t)

	tests := []struct {
		url  string
		want string
	}{
		{"/", "Frequently Asked Questions"},
		{"/", "Is the meat Halal?"},
		{"/", "application/ld+json"},
		{"/", "Privacy Policy"},
		{"/privacy", "Who we are"},
		{"/privacy", "Your rights"},
		{"/terms", "Refund policy (Halal guarantee)"},
	}
	for _, tc := range tests {
		resp, err := http.Get(srv.URL + tc.url)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.url, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("GET %s body: %v", tc.url, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", tc.url, resp.StatusCode)
		}
		if !strings.Contains(string(body), tc.want) {
			t.Fatalf("GET %s missing %q — layout or content not rendered", tc.url, tc.want)
		}
	}
}

// Pretty URLs are the only shareable filter surface (never ?query=).
func TestShopPrettyURLs(t *testing.T) {
	srv := newTestSite(t)

	tests := []struct {
		url    string
		status int
		want   string
	}{
		{"/shop", http.StatusOK, "Whole Chicken, cleaned"},
		{"/shop/meat", http.StatusOK, "Mutton Leg Cuts"},
		{"/shop/meat/madina", http.StatusOK, "Whole Chicken, cleaned"},
		{"/shop/meat/madina/1kg", http.StatusOK, "Mutton Leg Cuts"},
		{"/shop/grocery/al-safwa", http.StatusOK, "No products match"},
		{"/shop/electronics", http.StatusNotFound, "Page not found"},
		{"/shop/meat/madina/2kg", http.StatusNotFound, "Page not found"},
		{"/shop", http.StatusOK, `"ItemList"`},
		{"/shop", http.StatusOK, `"priceCurrency":"PKR"`},
	}
	for _, tc := range tests {
		resp, err := http.Get(srv.URL + tc.url)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.url, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("GET %s body: %v", tc.url, err)
		}
		if resp.StatusCode != tc.status {
			t.Fatalf("GET %s = %d, want %d", tc.url, resp.StatusCode, tc.status)
		}
		if !strings.Contains(string(body), tc.want) {
			t.Fatalf("GET %s missing %q", tc.url, tc.want)
		}
	}
}

// HTMX requests get the #shop-main fragment only (never a full page), and
// invalid filters under HTMX stay un-swapped with a 404.
func TestShopHXPartials(t *testing.T) {
	srv := newTestSite(t)

	req, err := http.NewRequest("GET", srv.URL+"/shop/meat", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("HX-Request", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HX GET /shop/meat: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HX GET /shop/meat = %d, want 200", resp.StatusCode)
	}
	if strings.Contains(string(body), "<!doctype") {
		t.Fatal("HX response must be a fragment, not a full page")
	}
	for _, want := range []string{`id="shop-main"`, "Mutton Leg Cuts", "chip-active"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("HX fragment missing %q", want)
		}
	}

	req2, _ := http.NewRequest("GET", srv.URL+"/shop/electronics", nil)
	req2.Header.Set("HX-Request", "true")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("HX invalid filter = %d, want 404", resp2.StatusCode)
	}
}

// Sitemap and robots.txt are generated from the live catalog.
func TestSitemapAndRobots(t *testing.T) {
	srv := newTestSite(t)

	resp, err := http.Get(srv.URL + "/sitemap.xml")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sitemap = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Fatalf("sitemap Content-Type = %q, want application/xml", ct)
	}
	for _, want := range []string{
		"<loc>http://localhost:3000/</loc>",
		"<loc>http://localhost:3000/privacy</loc>",
		"<loc>http://localhost:3000/terms</loc>",
		"<loc>http://localhost:3000/shop</loc>",
		"<loc>http://localhost:3000/shop/meat</loc>",
		"<loc>http://localhost:3000/shop/meat/madina</loc>",
		"<loc>http://localhost:3000/shop/meat/madina/1kg</loc>",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("sitemap missing %q", want)
		}
	}
	if strings.Contains(string(body), "/shop/grocery/al-safwa") {
		t.Fatal("empty filter combinations must not be listed in the sitemap")
	}

	resp2, err := http.Get(srv.URL + "/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("robots = %d, want 200", resp2.StatusCode)
	}
	for _, want := range []string{
		"User-agent: *",
		"Allow: /",
		"Disallow: /static/",
		"Sitemap: http://localhost:3000/sitemap.xml",
	} {
		if !strings.Contains(string(body2), want) {
			t.Fatalf("robots.txt missing %q", want)
		}
	}
}

func TestStaticDirectoryListingBlocked(t *testing.T) {
	srv := newTestSite(t)

	resp, err := http.Get(srv.URL + "/static/")
	if err != nil {
		t.Fatalf("GET /static/: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /static/ = %d, want 404 (no directory listings)", resp.StatusCode)
	}
}

func TestStaticFileStillServed(t *testing.T) {
	srv := newTestSite(t)

	resp, err := http.Get(srv.URL + "/static/styles.css")
	if err != nil {
		t.Fatalf("GET /static/styles.css: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/styles.css = %d, want 200", resp.StatusCode)
	}
}

// End-to-end: CSRF double-submit plus the cookie cart, with hardened
// cookies, the form-field fallback, HX badge swap and rejection paths.
func TestCSRFAndCartFlow(t *testing.T) {
	srv := newTestSite(t)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	base, _ := url.Parse(srv.URL)

	// Fresh POST with no cookies at all → 403.
	resp, err := client.Post(srv.URL+"/cart/add", "application/x-www-form-urlencoded",
		strings.NewReader("product_id=beef-1kg"))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF token = %d, want 403", resp.StatusCode)
	}

	// First GET issues the hardened cookies and embeds the token in the page.
	resp, err = client.Get(srv.URL + "/shop")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /shop = %d, want 200", resp.StatusCode)
	}
	var token string
	for _, c := range jar.Cookies(base) {
		if c.Name == "cm_csrf" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("GET must issue the cm_csrf cookie")
	}
	if !strings.Contains(string(body), "X-CSRF-Token") {
		t.Fatal("page must embed the CSRF token for hx-headers")
	}

	// Form-field fallback POST (no htmx) → 303 back to /shop; the cart cookie
	// must now exist (the cart lives in the signed cookie, not a session).
	resp, err = client.Post(srv.URL+"/cart/add", "application/x-www-form-urlencoded",
		strings.NewReader("product_id=beef-1kg&_csrf="+token))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("valid form-field POST = %d, want 303", resp.StatusCode)
	}
	var cartCookie string
	for _, c := range jar.Cookies(base) {
		if c.Name == "cm_cart" {
			cartCookie = c.Value
		}
	}
	if cartCookie == "" || !strings.Contains(cartCookie, ".") {
		t.Fatal("cart add must set the signed cm_cart cookie")
	}

	// The full page now shows the updated cart badge.
	resp, err = client.Get(srv.URL + "/shop")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Cart (1)") {
		t.Fatalf("cart badge not updated; body: %s", body)
	}

	// HX POST with the header token swaps the badge fragment.
	req, _ := http.NewRequest("POST", srv.URL+"/cart/add",
		strings.NewReader(url.Values{"product_id": {"beef-1kg"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-CSRF-Token", token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Cart (2)") {
		t.Fatalf("HX POST = %d, body %s; want fragment Cart (2)", resp.StatusCode, body)
	}

	// Wrong token → 403; unknown product with valid token → 400.
	for _, tc := range []struct {
		name, product, token string
		want                 int
	}{
		{"wrong token", "beef-1kg", "wrong-token", http.StatusForbidden},
		{"unknown product", "nope", token, http.StatusBadRequest},
	} {
		req, _ := http.NewRequest("POST", srv.URL+"/cart/add",
			strings.NewReader(url.Values{"product_id": {tc.product}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", tc.token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s: POST = %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}

// The catalog source fetches the API and degrades to the last good
// snapshot; a fresh process with a dead API renders an honestly empty shop.
func TestAPISourceDegradation(t *testing.T) {
	payload, _ := json.Marshal(testCatalog().products)

	ok := true
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products" {
			t.Errorf("unexpected API path %q", r.URL.Path)
		}
		if ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(payload)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(api.Close)

	src := NewAPISource(api.URL)
	products, err := src.All(context.Background())
	if err != nil || len(products) != 8 {
		t.Fatalf("first All = %d products, err %v; want 8, nil", len(products), err)
	}

	ok = false // API goes down
	products, err = src.All(context.Background())
	if err != nil || len(products) != 8 {
		t.Fatalf("stale All = %d products, err %v; want 8, nil", len(products), err)
	}

	// A fresh source with a dead API has nothing honest to show.
	fresh := NewAPISource(api.URL)
	products, err = fresh.All(context.Background())
	if !jsonIsEmpty(products) || err == nil {
		t.Fatalf("fresh source with dead API = %d products, err %v; want 0, error", len(products), err)
	}
}

func jsonIsEmpty(products []Product) bool {
	return len(products) == 0
}
