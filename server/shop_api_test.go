package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	// Registers the pure-Go sqlite driver for file::memory: DSNs.
	_ "modernc.org/sqlite"

	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/store"
)

// shopFlowServer boots the API over an in-memory store and registers one
// shop, returning the server and its bearer token — the base fixture for
// every shop-API test.
func shopFlowServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
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
	origDB, origCat, origPlaces, origSvc := dbStore, cat, places, orderSvc
	t.Cleanup(func() { dbStore, cat, places, orderSvc = origDB, origCat, origPlaces, origSvc })
	dbStore = st
	places = st
	orderSvc = orders.NewService(cat, places, st.DB()).WithStock(st)

	srv := httptest.NewServer(secureHeaders(newMux()))
	t.Cleanup(srv.Close)

	resp, m := postJSON(t, srv.URL+"/v1/auth/shop/register",
		`{"name":"Test Shelf","phone":"03211234567","password":"long-enough","lat":26.6931,"lng":67.6156}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("shop register = %d", resp.StatusCode)
	}
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatal("register must return a token")
	}
	return srv, token
}

// shopDo performs an authorized shop request and returns status + parsed body.
func shopDo(t *testing.T, srv *httptest.Server, token, method, path, body string) (int, map[string]any) {
	t.Helper()
	var req *http.Request
	var err error
	if body == "" {
		req, err = http.NewRequest(method, srv.URL+path, nil)
	} else {
		req, err = http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	}
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m
}

func TestShopProfileAndProductLifecycle(t *testing.T) {
	srv, token := shopFlowServer(t)

	// Profile: a fresh shop is unpaid, so ineligible for dispatch.
	code, m := shopDo(t, srv, token, "GET", "/v1/shop/me", "")
	if code != http.StatusOK || m["eligible"] != false || m["paid_through"] != "" {
		t.Fatalf("shop me = %d %v; want eligible=false, empty paid_through", code, m)
	}

	// Create a product on this shop's shelf.
	code, m = shopDo(t, srv, token, "POST", "/v1/shop/products",
		`{"name":"Sindh Rice 5kg","category":"grocery","brand":"noor","size":"5kg","price_minor":220000,"stock":10}`)
	if code != http.StatusCreated || m["id"] == nil {
		t.Fatalf("product create = %d %v", code, m)
	}
	pid, _ := m["id"].(string)

	// The shelf lists it with the live stock count.
	code, m = shopDo(t, srv, token, "GET", "/v1/shop/products", "")
	if code != http.StatusOK {
		t.Fatalf("shelf = %d", code)
	}
	if !strings.Contains(rawList(t, srv, token, "/v1/shop/products"), `"stock":10`) {
		t.Fatal("shelf must carry the stock count")
	}

	// The public catalog sees it immediately (snapshot reloaded on write).
	resp, err := http.Get(srv.URL + "/v1/products")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var list []catalog.Product
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range list {
		if p.ID == pid && p.Stock == 10 {
			found = true
		}
	}
	if !found {
		t.Fatal("public catalog must list the new product with live stock")
	}

	// Update the listing (price + shelf).
	code, _ = shopDo(t, srv, token, "PUT", "/v1/shop/products/"+pid,
		`{"name":"Sindh Rice 5kg","category":"grocery","brand":"noor","size":"5kg","price_minor":230000,"stock":8}`)
	if code != http.StatusOK {
		t.Fatalf("product update = %d", code)
	}

	// Another shop's product is a 404, not a leak.
	code, _ = shopDo(t, srv, token, "PUT", "/v1/shop/products/beef-1kg",
		`{"name":"Stolen","category":"meat","brand":"x","size":"1kg","price_minor":100,"stock":1}`)
	if code != http.StatusNotFound {
		t.Fatalf("foreign product update = %d, want 404", code)
	}

	// Validation: bad price, negative stock.
	code, _ = shopDo(t, srv, token, "POST", "/v1/shop/products",
		`{"name":"X","category":"grocery","brand":"noor","size":"1kg","price_minor":10,"stock":1}`)
	if code != http.StatusBadRequest {
		t.Fatalf("underpriced product = %d, want 400", code)
	}
	code, _ = shopDo(t, srv, token, "POST", "/v1/shop/products",
		`{"name":"X","category":"grocery","brand":"noor","size":"1kg","price_minor":100,"stock":-1}`)
	if code != http.StatusBadRequest {
		t.Fatalf("negative stock = %d, want 400", code)
	}
}

func rawList(t *testing.T, srv *httptest.Server, token, path string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(raw)
}

func TestShopAcceptDecrementsStockAndCancelRestores(t *testing.T) {
	srv, token := shopFlowServer(t)

	// The shop lists beef at qty 5.
	code, m := shopDo(t, srv, token, "POST", "/v1/shop/products",
		`{"name":"Beef Boneless 1kg","category":"meat","brand":"al-safwa","size":"1kg","price_minor":140000,"stock":5}`)
	if code != http.StatusCreated {
		t.Fatalf("product create = %d %v", code, m)
	}
	pid, _ := m["id"].(string)

	// A citizen orders 3 units.
	resp, raw := postOrder(t, srv.URL, `{"items":[{"product_id":"`+pid+`","qty":3}],"lat":26.6931,"lng":67.6156}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("place = %d, want 201; body %s", resp.StatusCode, raw)
	}
	var o struct {
		ID     string `json:"id"`
		ShopID string `json:"shop_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatal(err)
	}
	if o.ShopID == "" || !strings.HasPrefix(o.ShopID, "sh-") {
		t.Fatalf("order must target the registered shop, got %+v", o)
	}

	// Accept: stock drops 5 → 2 and the order confirms.
	code, m = shopDo(t, srv, token, "POST", "/v1/shop/orders/"+o.ID+"/accept", "")
	if code != http.StatusOK || m["status"] != "confirmed" {
		t.Fatalf("accept = %d %v", code, m)
	}
	if shelf(t, srv, token, pid) != 2 {
		t.Fatal("accept must decrement the shelf by the ordered quantity")
	}

	// Accepting again is a 409 (already confirmed), never a second decrement.
	code, _ = shopDo(t, srv, token, "POST", "/v1/shop/orders/"+o.ID+"/accept", "")
	if code != http.StatusConflict {
		t.Fatalf("double accept = %d, want 409", code)
	}
	if shelf(t, srv, token, pid) != 2 {
		t.Fatal("a double accept must not decrement twice")
	}

	// Ordering more than the shelf holds is refused at placement.
	resp, _ = postOrder(t, srv.URL, `{"items":[{"product_id":"`+pid+`","qty":99}],"lat":26.6931,"lng":67.6156}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("over-stock placement = %d, want 409", resp.StatusCode)
	}

	// Cancel the confirmed order: the 3 units go back on the shelf.
	code, m = shopDo(t, srv, token, "POST", "/v1/shop/orders/"+o.ID+"/cancel", "")
	if code != http.StatusOK || m["status"] != "cancelled" {
		t.Fatalf("cancel = %d %v", code, m)
	}
	if shelf(t, srv, token, pid) != 5 {
		t.Fatal("cancelling a confirmed order must restore the shelf")
	}
}

// shelf reads one product's stock through the shop's own listing API.
func shelf(t *testing.T, srv *httptest.Server, token, productID string) int64 {
	t.Helper()
	raw := rawList(t, srv, token, "/v1/shop/products")
	var list []catalog.Product
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ID == productID {
			return p.Stock
		}
	}
	t.Fatalf("product %s missing from shelf", productID)
	return 0
}

func postOrder(t *testing.T, base, body string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Post(base+"/v1/orders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, raw
}

func TestShopQueueRequiresTheShopToken(t *testing.T) {
	srv, token := shopFlowServer(t)

	// No token → 401 from the middleware.
	resp, err := http.Get(srv.URL + "/v1/shop/orders")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("queue without token = %d, want 401", resp.StatusCode)
	}

	// A rider token is not a shop token (role isolation at the route level).
	resp, m := postJSON(t, srv.URL+"/v1/auth/rider/register",
		`{"name":"Imran","phone":"03459999999","password":"long-enough","lat":26.694,"lng":67.616}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("rider register = %d", resp.StatusCode)
	}
	riderToken, _ := m["token"].(string)
	code, _ := shopDo(t, srv, riderToken, "GET", "/v1/shop/me", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("rider token on shop API = %d, want 401", code)
	}

	// The shop's own queue works and starts empty.
	code, m = shopDo(t, srv, token, "GET", "/v1/shop/orders", "")
	if code != http.StatusOK {
		t.Fatalf("queue = %d", code)
	}
	if m["error"] != nil {
		t.Fatalf("queue errored: %v", m)
	}
}
