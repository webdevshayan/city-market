package main

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	// Registers the pure-Go sqlite driver for file::memory: DSNs.
	_ "modernc.org/sqlite"

	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/ratelimit"
	"github.com/webdevshayan/city-market/server/store"
)

// ordersForTest wires the engine with the store as stock and eligibility source.
func ordersForTest(st *store.Store) *orders.Service {
	return orders.NewService(cat, places, st.DB()).WithStock(st).WithEligibility(st)
}

// riderFlowServer boots the API over an in-memory store with one paid shop,
// registers that shop and one rider, and returns both tokens.
func riderFlowServer(t *testing.T) (*httptest.Server, string, string, string) {
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
	orderSvc = ordersForTest(st)

	srv := httptest.NewServer(secureHeaders(newMuxWith(
		ratelimit.New(1000, 10000).Middleware,
		ratelimit.New(1000, 10000).Middleware)))
	t.Cleanup(srv.Close)

	// The paid shop keeps its seeded shelf (beef-1kg et al).
	resp, m := postJSON(t, srv.URL+"/v1/auth/shop/register",
		`{"name":"Claim Shop","phone":"03331111111","password":"long-enough","lat":26.6931,"lng":67.6156}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("shop register = %d", resp.StatusCode)
	}
	shopID, _ := m["shop"].(map[string]any)["id"].(string)
	if err := dbStore.SetShopPaid(shopID, "2099-12", "test seed"); err != nil {
		t.Fatal(err)
	}
	shopToken, _ := m["token"].(string)
	// The seeded shops hold the launch shelf; pay s-central so the citizen
	// orders in these tests have an eligible owner.
	if err := dbStore.SetShopPaid("s-central", "2099-12", "test seed"); err != nil {
		t.Fatal(err)
	}

	resp, m = postJSON(t, srv.URL+"/v1/auth/rider/register",
		`{"name":"Board Rider","phone":"03441111111","password":"long-enough","lat":26.6940,"lng":67.6160}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("rider register = %d", resp.StatusCode)
	}
	riderToken, _ := m["token"].(string)

	// The flow shop lists its own product so every order in these tests
	// belongs to a shop whose token we hold (the seeded shops have none).
	code, m, _ := bearerDo(t, srv, shopToken, "POST", "/v1/shop/products",
		`{"name":"Claim Bread","category":"bakery","brand":"barakah","size":"10pcs","price_minor":35000,"stock":5}`)
	if code != http.StatusCreated {
		t.Fatalf("flow product create = %d %v", code, m)
	}
	pid, _ := m["id"].(string)

	return srv, shopToken, riderToken, pid
}

// bearerDo performs an authorized request; body "" means no payload.
func bearerDo(t *testing.T, srv *httptest.Server, token, method, path, body string) (int, map[string]any, []byte) {
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
	return resp.StatusCode, m, raw
}

func TestRiderDutyAndBoard(t *testing.T) {
	srv, shopToken, riderToken, pid := riderFlowServer(t)

	// On duty at the rider's registered spot.
	code, m, _ := bearerDo(t, srv, riderToken, "POST", "/v1/rider/me",
		`{"lat":26.6940,"lng":67.6160,"available":true}`)
	if code != http.StatusOK || m["available"] != true {
		t.Fatalf("rider me = %d %v", code, m)
	}

	// Nothing accepted yet → an empty board.
	code, _, raw := bearerDo(t, srv, riderToken, "GET", "/v1/rider/board", "")
	if code != http.StatusOK || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("empty board = %d %s, want []", code, raw)
	}

	// A citizen orders from the paid shop; the shop accepts → the job is
	// open on the board with the full estimate set.
	resp, raw := postOrder(t, srv.URL, `{"items":[{"product_id":"`+pid+`","qty":1}],"lat":26.7,"lng":67.65,"address_note":"Near the grain market, blue shutter"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("place = %d, want 201; body %s", resp.StatusCode, raw)
	}
	var placed struct {
		ID     string `json:"id"`
		ShopID string `json:"shop_id"`
	}
	if err := json.Unmarshal(raw, &placed); err != nil {
		t.Fatal(err)
	}

	code, m, _ = bearerDo(t, srv, shopToken, "POST", "/v1/shop/orders/"+placed.ID+"/accept", "")
	if code != http.StatusOK || m["status"] != "confirmed" {
		t.Fatalf("accept = %d %v", code, m)
	}

	code, _, raw = bearerDo(t, srv, riderToken, "GET", "/v1/rider/board", "")
	if code != http.StatusOK {
		t.Fatalf("board = %d", code)
	}
	var board []map[string]any
	if err := json.Unmarshal(raw, &board); err != nil {
		t.Fatal(err)
	}
	if len(board) != 1 {
		t.Fatalf("board = %d jobs, want the one accepted order", len(board))
	}
	job := board[0]
	for _, field := range []string{"order_id", "shop_name", "shop_lat", "shop_lng", "drop_lat", "drop_lng", "address_note", "pickup_km", "route_km", "eta_minutes", "payout_minor"} {
		if _, ok := job[field]; !ok {
			t.Fatalf("board row missing %q: %v", field, job)
		}
	}
	if job["address_note"] != "Near the grain market, blue shutter" {
		t.Fatalf("address note = %v, want the citizen's delivery address", job["address_note"])
	}
	if job["eta_minutes"].(float64) <= 0 || job["route_km"].(float64) < 0 {
		t.Fatalf("estimates = %v, want positive ETA and non-negative route", job)
	}
	if job["payout_minor"].(float64) < 6000 {
		t.Fatalf("payout = %v, want at least the 60.00 base", job["payout_minor"])
	}
}

func TestRiderClaimLifecycle(t *testing.T) {
	srv, shopToken, riderToken, pid := riderFlowServer(t)

	// On duty first — an off-duty rider cannot claim.
	code, m, _ := bearerDo(t, srv, riderToken, "POST", "/v1/rider/me",
		`{"lat":26.6940,"lng":67.6160,"available":false}`)
	if code != http.StatusOK {
		t.Fatal(code)
	}
	resp, raw := postOrder(t, srv.URL, `{"items":[{"product_id":"`+pid+`","qty":1}],"lat":26.7,"lng":67.65}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("place = %d; body %s", resp.StatusCode, raw)
	}
	var placed struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &placed)
	code, _, _ = bearerDo(t, srv, shopToken, "POST", "/v1/shop/orders/"+placed.ID+"/accept", "")
	if code != http.StatusOK {
		t.Fatalf("accept = %d", code)
	}

	// Off duty → the claim is refused.
	code, m, _ = bearerDo(t, srv, riderToken, "POST", "/v1/rider/orders/"+placed.ID+"/claim", "")
	if code != http.StatusConflict || m["error"] == nil {
		t.Fatalf("off-duty claim = %d %v, want 409", code, m)
	}

	// On duty → the claim locks the job to this rider.
	code, _, _ = bearerDo(t, srv, riderToken, "POST", "/v1/rider/me",
		`{"lat":26.6940,"lng":67.6160,"available":true}`)
	if code != http.StatusOK {
		t.Fatal(code)
	}
	code, m, _ = bearerDo(t, srv, riderToken, "POST", "/v1/rider/orders/"+placed.ID+"/claim", "")
	if code != http.StatusOK || m["rider_id"] == nil || m["rider_id"] == "" {
		t.Fatalf("claim = %d %v, want the rider assigned", code, m)
	}
	if m["claimed_at"] == "" || m["claimed_at"] == nil {
		t.Fatalf("claim must stamp claimed_at, got %v", m["claimed_at"])
	}
	payout := m["rider_payout_minor"].(float64)
	if payout != math.Trunc(payout) || payout < 6000 {
		t.Fatalf("locked payout = %v, want the base or more", payout)
	}

	// The claimed job left the board.
	_, _, raw2 := bearerDo(t, srv, riderToken, "GET", "/v1/rider/board", "")
	if strings.TrimSpace(string(raw2)) != "[]" {
		t.Fatalf("board after claim = %s, want []", raw2)
	}

	// The rider's own job list carries it, and only its owner can advance it.
	code, _, raw = bearerDo(t, srv, riderToken, "GET", "/v1/rider/orders", "")
	if code != http.StatusOK || !strings.Contains(string(raw), placed.ID) {
		t.Fatalf("rider jobs = %d %s", code, raw)
	}

	// A second rider may not touch someone else's job.
	resp, m2 := postJSON(t, srv.URL+"/v1/auth/rider/register",
		`{"name":"Second Rider","phone":"03442222222","password":"long-enough","lat":26.69,"lng":67.61}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second rider = %d %v", resp.StatusCode, m2)
	}
	other, _ := m2["token"].(string)
	code, _, _ = bearerDo(t, srv, other, "POST", "/v1/rider/orders/"+placed.ID+"/pick-up", "")
	if code != http.StatusNotFound {
		t.Fatalf("foreign pick-up = %d, want 404", code)
	}

	// The owner walks the job through pick-up → deliver.
	code, m, _ = bearerDo(t, srv, riderToken, "POST", "/v1/rider/orders/"+placed.ID+"/pick-up", "")
	if code != http.StatusOK || m["status"] != "picked_up" {
		t.Fatalf("pick-up = %d %v", code, m)
	}
	code, m, _ = bearerDo(t, srv, riderToken, "POST", "/v1/rider/orders/"+placed.ID+"/deliver", "")
	if code != http.StatusOK || m["status"] != "delivered" {
		t.Fatalf("deliver = %d %v", code, m)
	}

	// A repeat deliver is a 409 — the ledger never double-counts.
	code, _, _ = bearerDo(t, srv, riderToken, "POST", "/v1/rider/orders/"+placed.ID+"/deliver", "")
	if code != http.StatusConflict {
		t.Fatalf("repeat deliver = %d, want 409", code)
	}
}

func TestRiderClaimRaceIsFirstWins(t *testing.T) {
	srv, shopToken, riderToken, pid := riderFlowServer(t)

	resp, raw := postOrder(t, srv.URL, `{"items":[{"product_id":"`+pid+`","qty":1}],"lat":26.7,"lng":67.65}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("place = %d", resp.StatusCode)
	}
	var placed struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &placed)
	if code, _, _ := bearerDo(t, srv, shopToken, "POST", "/v1/shop/orders/"+placed.ID+"/accept", ""); code != http.StatusOK {
		t.Fatalf("accept = %d", code)
	}

	// Two riders both on duty; only the first claim wins.
	bearerDo(t, srv, riderToken, "POST", "/v1/rider/me",
		`{"lat":26.6940,"lng":67.6160,"available":true}`)
	resp, m2 := postJSON(t, srv.URL+"/v1/auth/rider/register",
		`{"name":"Racer","phone":"03443333333","password":"long-enough","lat":26.6931,"lng":67.6156}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(resp.StatusCode)
	}
	racer, _ := m2["token"].(string)
	if code, _, _ := bearerDo(t, srv, racer, "POST", "/v1/rider/me",
		`{"lat":26.6931,"lng":67.6156,"available":true}`); code != http.StatusOK {
		t.Fatal(code)
	}

	code1, _, _ := bearerDo(t, srv, riderToken, "POST", "/v1/rider/orders/"+placed.ID+"/claim", "")
	code2, m3, _ := bearerDo(t, srv, racer, "POST", "/v1/rider/orders/"+placed.ID+"/claim", "")
	if !(code1 == http.StatusOK && code2 == http.StatusConflict) &&
		!(code2 == http.StatusOK && code1 == http.StatusConflict) {
		t.Fatalf("claim race = %d/%d %v — exactly one must win", code1, code2, m3)
	}
}
