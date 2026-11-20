package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	// Registers the pure-Go sqlite driver so file::memory: DSNs work in the
	// DB-backed API tests (libsql itself registers through the store package).
	_ "modernc.org/sqlite"

	"github.com/webdevshayan/city-market/server/csrf"
	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/ratelimit"
	"github.com/webdevshayan/city-market/server/store"
)

// Review finding M2: directory listings must be blocked; real files stay up.
func TestStaticDirectoryListingBlocked(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

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
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/static/styles.css")
	if err != nil {
		t.Fatalf("GET /static/styles.css: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/styles.css = %d, want 200", resp.StatusCode)
	}
}

// Regression for the blank-page bug: a page set renders its "base" layout,
// so every page must actually contain layout and page content.
func TestPagesRenderFullLayout(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

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
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

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
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

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

// Sitemap and robots.txt are generated from the catalog.
func TestSitemapAndRobots(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

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

// End-to-end: CSRF double-submit protection plus the cart flow, with cookie
// hardening, form-field fallback, HX badge swap and rejection paths.
func TestCSRFAndCartFlow(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(csrf.Middleware(sessionCookie(newMux()))))
	defer srv.Close()

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
	resp, err := client.PostForm(srv.URL+"/cart/add", url.Values{"product_id": {"beef-1kg"}})
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

	// Form-field fallback POST (no htmx) → 303 back to /shop.
	resp, err = client.PostForm(srv.URL+"/cart/add", url.Values{"product_id": {"beef-1kg"}, "_csrf": {token}})
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("valid form-field POST = %d, want 303", resp.StatusCode)
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
	req, _ := http.NewRequest("POST", srv.URL+"/cart/add", strings.NewReader(url.Values{"product_id": {"beef-1kg"}}.Encode()))
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
		req, _ := http.NewRequest("POST", srv.URL+"/cart/add", strings.NewReader(url.Values{"product_id": {tc.product}}.Encode()))
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

// Phase 3 matching endpoints: nearest active shop/rider over the seed
// fixtures (r-4 sits at the origin but is off duty — the skip-inactive path
// is exercised through the API), product-restricted dispatch, and validation.
func TestMatchEndpoints(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	tests := []struct {
		name   string
		url    string
		status int
		want   string
	}{
		{"nearest shop at the central coords", "/api/v1/match/shop?lat=24.8607&lng=67.0011", 200, `"id":"s-central"`},
		{"nearest available rider", "/api/v1/match/rider?lat=24.8607&lng=67.0011", 200, `"id":"r-1"`},
		{"product-restricted shop despite distance", "/api/v1/match/shop?lat=24.8138&lng=67.0300&product=beef-1kg", 200, `"id":"s-central"`},
		{"unknown product", "/api/v1/match/shop?lat=24.86&lng=67.00&product=nope", 404, "unknown product"},
		{"missing coordinates", "/api/v1/match/shop", 400, "missing required query parameters"},
		{"out-of-range latitude", "/api/v1/match/rider?lat=200&lng=67", 400, "within [-90, 90]"},
		{"non-numeric longitude", "/api/v1/match/rider?lat=24&lng=abc", 400, "decimal degrees"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.url)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("GET %s = %d, want %d", tc.url, resp.StatusCode, tc.status)
			}
			if !strings.Contains(string(body), tc.want) {
				t.Fatalf("GET %s missing %q", tc.url, tc.want)
			}
		})
	}

	resp, err := http.Get(srv.URL + "/api/v1/match/shop?lat=24.8607&lng=67.0011")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"distance_meters":0`) {
		t.Fatalf("distance at the shop's own coordinates must be 0; body: %s", body)
	}
}

// The public API is rate limited per IP; a tiny burst makes the 429
// deterministic.
func TestPublicAPIRateLimited(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMuxWith(
		ratelimit.New(1, 2).Middleware,
		ratelimit.New(1, 2).Middleware,
	)))
	defer srv.Close()

	for i := 0; i < 2; i++ {
		resp, err := http.Get(srv.URL + "/api/v1/finance/checkout-fee")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i+1, resp.StatusCode)
		}
	}
	resp, err := http.Get(srv.URL + "/api/v1/finance/checkout-fee")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third request = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

// Without a database the citizen order API must say so honestly (503) —
// never fabricate an order it cannot persist.
func TestOrderPlacementAPINoDatabase(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/orders", "application/json",
		strings.NewReader(`{"items":[{"product_id":"beef-1kg","qty":1}],"lat":24.86,"lng":67.0}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("no-DB placement = %d, want 503", resp.StatusCode)
	}
}

// With the database wired, the citizen flow works end to end over the API:
// place (priced + assigned), re-read, and the error paths.
func TestOrderPlacementAPIWithDatabase(t *testing.T) {
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedIfEmpty(store.SeedShops(), store.SeedRiders(), store.SeedProducts()); err != nil {
		t.Fatal(err)
	}
	orderSvc = orders.NewService(cat, places, st.DB())
	t.Cleanup(func() { orderSvc = nil }) // tests run sequentially; restore the no-DB default

	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	post := func(body string) (*http.Response, []byte) {
		resp, err := http.Post(srv.URL+"/api/v1/orders", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}

	resp, raw := post(`{"items":[{"product_id":"beef-1kg","qty":2}],"lat":24.8607,"lng":67.0011}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("place = %d, want 201; body %s", resp.StatusCode, raw)
	}
	var o orders.Order
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if o.Status != orders.StatusPlaced || o.ShopID != "s-central" || o.RiderID != "r-1" {
		t.Fatalf("order = %+v", o)
	}
	if o.TotalMinor != 282500 { // 2 × 1400.00 + the 25.00 flat delivery fee
		t.Fatalf("total = %d, want 282500", o.TotalMinor)
	}
	if o.RiderPayoutMinor != 6000 { // shop at the customer's own coordinates: 0 km route
		t.Fatalf("payout = %d, want 6000", o.RiderPayoutMinor)
	}

	resp2, err := http.Get(srv.URL + "/api/v1/orders/" + o.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("get order = %d, want 200", resp2.StatusCode)
	}
	var got orders.Order
	if err := json.Unmarshal(raw2, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != o.ID || got.TotalMinor != o.TotalMinor {
		t.Fatal("re-read order must match")
	}

	cases := []struct {
		name, body string
		want       int
	}{
		{"unknown product", `{"items":[{"product_id":"nope","qty":1}],"lat":24.86,"lng":67}`, 400},
		{"multi-shop cart", `{"items":[{"product_id":"beef-1kg","qty":1},{"product_id":"rice-5kg","qty":1}],"lat":24.86,"lng":67}`, 409},
		{"empty items", `{"items":[],"lat":24.86,"lng":67}`, 400},
		{"bad coordinates", `{"items":[{"product_id":"beef-1kg","qty":1}],"lat":999,"lng":67}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := post(tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("= %d, want %d; body %s", resp.StatusCode, tc.want, raw)
			}
		})
	}

	resp3, err := http.Get(srv.URL + "/api/v1/orders/ord-missing")
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown order = %d, want 404", resp3.StatusCode)
	}
}

// Shop and rider order APIs: queue lists and the guarded status steps.
func TestShopAndRiderOrderAPIs(t *testing.T) {
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedIfEmpty(store.SeedShops(), store.SeedRiders(), store.SeedProducts()); err != nil {
		t.Fatal(err)
	}
	orderSvc = orders.NewService(cat, places, st.DB())
	t.Cleanup(func() { orderSvc = nil })

	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	place := func(body string) orders.Order {
		resp, err := http.Post(srv.URL+"/api/v1/orders", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("place = %d, want 201; body %s", resp.StatusCode, raw)
		}
		var o orders.Order
		if err := json.Unmarshal(raw, &o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	o1 := place(`{"items":[{"product_id":"beef-1kg","qty":1}],"lat":24.8607,"lng":67.0011}`)
	o2 := place(`{"items":[{"product_id":"naan-10pcs","qty":1}],"lat":24.8607,"lng":67.0011}`)

	get := func(url string) (*http.Response, []byte) {
		resp, err := http.Get(srv.URL + url)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}

	resp, raw := get("/api/v1/shop/orders?shop_id=s-central")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("shop queue = %d, want 200; body %s", resp.StatusCode, raw)
	}
	var shopQueue []orders.Order
	if err := json.Unmarshal(raw, &shopQueue); err != nil {
		t.Fatal(err)
	}
	if len(shopQueue) < 2 {
		t.Fatalf("shop queue = %d orders, want >= 2", len(shopQueue))
	}

	resp, _ = get("/api/v1/rider/orders?rider_id=r-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rider queue = %d, want 200", resp.StatusCode)
	}
	resp, _ = get("/api/v1/shop/orders")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing shop_id = %d, want 400", resp.StatusCode)
	}

	post := func(url string) (*http.Response, []byte) {
		resp, err := http.Post(srv.URL+url, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}

	steps := []struct {
		url  string
		want string
	}{
		{"/api/v1/shop/orders/" + o1.ID + "/confirm", string(orders.StatusConfirmed)},
		{"/api/v1/rider/orders/" + o1.ID + "/pick-up", string(orders.StatusPickedUp)},
		{"/api/v1/rider/orders/" + o1.ID + "/deliver", string(orders.StatusDelivered)},
	}
	for _, st := range steps {
		t.Run(st.want, func(t *testing.T) {
			resp, raw := post(st.url)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s = %d, want 200; body %s", st.url, resp.StatusCode, raw)
			}
			var o orders.Order
			if err := json.Unmarshal(raw, &o); err != nil {
				t.Fatal(err)
			}
			if string(o.Status) != st.want {
				t.Fatalf("status = %q, want %q", o.Status, st.want)
			}
		})
	}

	// A repeat or skipped step must conflict, not silently change state.
	resp, _ = post("/api/v1/rider/orders/" + o1.ID + "/deliver")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delivered → deliver again = %d, want 409", resp.StatusCode)
	}
	resp, _ = post("/api/v1/rider/orders/" + o2.ID + "/deliver")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("placed → deliver (skipped steps) = %d, want 409", resp.StatusCode)
	}
	resp, _ = post("/api/v1/shop/orders/ord-missing/confirm")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown order confirm = %d, want 404", resp.StatusCode)
	}
}
