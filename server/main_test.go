package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	// Registers the pure-Go sqlite driver so file::memory: DSNs work in the
	// DB-backed API tests (libsql itself registers through the store package).
	_ "modernc.org/sqlite"

	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/ratelimit"
	"github.com/webdevshayan/city-market/server/store"
)

// Review finding M2: directory listings must be blocked; real files stay up.
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
		{"nearest shop at Johi center", "/v1/match/shop?lat=26.6921&lng=67.6133", 200, `"id":"s-central"`},
		{"nearest available rider", "/v1/match/rider?lat=26.6921&lng=67.6133", 200, `"id":"r-1"`},
		{"product-restricted shop despite distance", "/v1/match/shop?lat=26.7300&lng=67.7769&product=beef-1kg", 200, `"id":"s-central"`},
		{"unknown product", "/v1/match/shop?lat=24.86&lng=67.00&product=nope", 404, "unknown product"},
		{"missing coordinates", "/v1/match/shop", 400, "missing required query parameters"},
		{"out-of-range latitude", "/v1/match/rider?lat=200&lng=67", 400, "within [-90, 90]"},
		{"non-numeric longitude", "/v1/match/rider?lat=24&lng=abc", 400, "decimal degrees"},
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

	resp, err := http.Get(srv.URL + "/v1/match/shop?lat=26.6931&lng=67.6156")
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
		resp, err := http.Get(srv.URL + "/v1/finance/checkout-fee")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i+1, resp.StatusCode)
		}
	}
	resp, err := http.Get(srv.URL + "/v1/finance/checkout-fee")
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

	resp, err := http.Post(srv.URL+"/v1/orders", "application/json",
		strings.NewReader(`{"items":[{"product_id":"beef-1kg","qty":1}],"lat":26.69,"lng":67.61}`))
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
		resp, err := http.Post(srv.URL+"/v1/orders", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}

	resp, raw := post(`{"items":[{"product_id":"beef-1kg","qty":2}],"lat":26.6931,"lng":67.6156}`)
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

	resp2, err := http.Get(srv.URL + "/v1/orders/" + o.ID)
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
		{"unknown product", `{"items":[{"product_id":"nope","qty":1}],"lat":26.69,"lng":67.61}`, 400},
		{"multi-shop cart", `{"items":[{"product_id":"beef-1kg","qty":1},{"product_id":"rice-5kg","qty":1}],"lat":26.69,"lng":67.61}`, 409},
		{"empty items", `{"items":[],"lat":26.69,"lng":67.61}`, 400},
		{"bad coordinates", `{"items":[{"product_id":"beef-1kg","qty":1}],"lat":999,"lng":67.61}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := post(tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("= %d, want %d; body %s", resp.StatusCode, tc.want, raw)
			}
		})
	}

	resp3, err := http.Get(srv.URL + "/v1/orders/ord-missing")
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
		resp, err := http.Post(srv.URL+"/v1/orders", "application/json", strings.NewReader(body))
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
	o1 := place(`{"items":[{"product_id":"beef-1kg","qty":1}],"lat":26.6931,"lng":67.6156}`)
	o2 := place(`{"items":[{"product_id":"naan-10pcs","qty":1}],"lat":26.6931,"lng":67.6156}`)

	get := func(url string) (*http.Response, []byte) {
		resp, err := http.Get(srv.URL + url)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}

	resp, raw := get("/v1/shop/orders?shop_id=s-central")
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

	resp, _ = get("/v1/rider/orders?rider_id=r-1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rider queue = %d, want 200", resp.StatusCode)
	}
	resp, _ = get("/v1/shop/orders")
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
		{"/v1/shop/orders/" + o1.ID + "/confirm", string(orders.StatusConfirmed)},
		{"/v1/rider/orders/" + o1.ID + "/pick-up", string(orders.StatusPickedUp)},
		{"/v1/rider/orders/" + o1.ID + "/deliver", string(orders.StatusDelivered)},
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
	resp, _ = post("/v1/rider/orders/" + o1.ID + "/deliver")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delivered → deliver again = %d, want 409", resp.StatusCode)
	}
	resp, _ = post("/v1/rider/orders/" + o2.ID + "/deliver")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("placed → deliver (skipped steps) = %d, want 409", resp.StatusCode)
	}
	resp, _ = post("/v1/shop/orders/ord-missing/confirm")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown order confirm = %d, want 404", resp.StatusCode)
	}
}

// The admin console is default-deny: an unset (or empty) ADMIN_TOKEN
// disables it entirely.
func TestAdminAPIDisabledWithoutToken(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/admin/products")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("admin without token = %d, want 503 (disabled)", resp.StatusCode)
	}
}

// Admin API: constant-time token auth, live dispatch/catalog writes and the
// commission settlement view.
func TestAdminAPIAuthAndUpserts(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "test-secret")

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
	// Preserve the no-DB defaults for later tests.
	origCat, origPlaces, origDB := cat, places, dbStore
	t.Cleanup(func() { cat, places, dbStore = origCat, origPlaces, origDB; orderSvc = nil })
	dbStore = st
	places = st

	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	do := func(method, url, token, body string) (*http.Response, []byte) {
		var req *http.Request
		var err error
		if body == "" {
			req, err = http.NewRequest(method, srv.URL+url, nil)
		} else {
			req, err = http.NewRequest(method, srv.URL+url, strings.NewReader(body))
		}
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("X-Admin-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, raw
	}

	// Wrong token → 401 (throttled by the rate limiter wrapping auth).
	resp, _ := do("GET", "/v1/admin/products", "wrong-token", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong admin token = %d, want 401", resp.StatusCode)
	}

	// Upsert a shop → dispatch serves it live.
	resp, raw := do("PUT", "/v1/admin/shops/s-dha", "test-secret",
		`{"name":"Dadu Road Mart","lat":26.7300,"lng":67.7769,"active":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert shop = %d, want 200; body %s", resp.StatusCode, raw)
	}

	resp, raw = do("GET", "/v1/match/shop?lat=26.7300&lng=67.7769", "", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"id":"s-dha"`) {
		t.Fatalf("dispatch after admin upsert = %d %s; want s-dha", resp.StatusCode, raw)
	}

	// Upsert a product → catalog (and order service) reload live.
	resp, raw = do("PUT", "/v1/admin/products/gajar-1kg", "test-secret",
		`{"name":"Gajar ka Halal Mix","category":"grocery","brand":"noor","size":"1kg","price_minor":50000,"image":"/static/img/grocery.svg","shop_id":"s-dha"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert product = %d, want 200; body %s", resp.StatusCode, raw)
	}
	resp, raw = do("GET", "/v1/admin/products", "test-secret", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "gajar-1kg") {
		t.Fatalf("admin product list = %d %s; want gajar-1kg (live reload)", resp.StatusCode, raw)
	}

	resp, raw = do("POST", "/v1/orders", "",
		`{"items":[{"product_id":"gajar-1kg","qty":1}],"lat":26.7300,"lng":67.7769}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("order for the new product = %d, want 201; body %s", resp.StatusCode, raw)
	}
	var o orders.Order
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatal(err)
	}
	if o.ShopID != "s-dha" {
		t.Fatalf("order shop = %q, want s-dha (catalog reloaded)", o.ShopID)
	}

	// Commission settlement view — 250 orders @ 700 bps on 100,000 major.
	resp, raw = do("GET", "/v1/admin/commission?orders=250&sales=10000000", "test-secret", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"commission_rate_bps":700`) || !strings.Contains(string(raw), `"commission_minor":700000`) {
		t.Fatalf("commission = %d %s", resp.StatusCode, raw)
	}
	resp, _ = do("GET", "/v1/admin/commission?orders=abc&sales=1", "test-secret", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("commission bad input = %d, want 400", resp.StatusCode)
	}

	// Malformed admin bodies are rejected loudly.
	resp, _ = do("PUT", "/v1/admin/shops/s-bad", "test-secret", `{"nope":1}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown admin field = %d, want 400", resp.StatusCode)
	}
	resp, _ = do("PUT", "/v1/admin/shops/s-bad", "test-secret", `{"name":"X","lat":999,"lng":0,"active":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("admin out-of-range coords = %d, want 400", resp.StatusCode)
	}
}

// The public catalog endpoint feeds the standalone web app (separate
// deployment, own domain): the same JSON the admin list serves, but
// unauthenticated and rate-limited like every public read.
func TestPublicProductsAPI(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/products")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("products = %d, want 200; body %s", resp.StatusCode, raw)
	}
	var list []catalog.Product
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode products: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("seed catalog must not be empty")
	}
	found := false
	for _, p := range list {
		if p.ID == "beef-1kg" && p.PriceMinor == 140000 && p.ShopID == "s-central" {
			found = true
		}
	}
	if !found {
		t.Fatalf("seed products missing beef-1kg: %s", raw)
	}
}

// The API root is a discoverable index, and unmatched paths answer JSON —
// this deployment serves no HTML pages.
func TestAPIIndexAndJSON404(t *testing.T) {
	srv := httptest.NewServer(secureHeaders(newMux()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index = %d, want 200; body %s", resp.StatusCode, raw)
	}
	var idx map[string]any
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("decode index: %v", err)
	}
	if idx["service"] != "city-market API" {
		t.Fatalf("index service = %v", idx["service"])
	}

	resp2, err := http.Get(srv.URL + "/definitely-not-a-page")
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path = %d, want 404", resp2.StatusCode)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw2, &e); err != nil || !strings.Contains(e.Error, "unknown API path") {
		t.Fatalf("404 must be JSON with a helpful error; got %s", raw2)
	}
}
