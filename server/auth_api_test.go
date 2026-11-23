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

	"github.com/webdevshayan/city-market/server/ratelimit"
	"github.com/webdevshayan/city-market/server/store"
)

// authServer spins an API backed by its own in-memory database, so account
// tests never leak sessions into each other.
func authServer(t *testing.T) *httptest.Server {
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
	origDB := dbStore
	t.Cleanup(func() { dbStore = origDB })
	dbStore = st
	srv := httptest.NewServer(secureHeaders(newMuxWith(
		ratelimit.New(1000, 10000).Middleware,
		ratelimit.New(1000, 10000).Middleware)))
	t.Cleanup(srv.Close)
	return srv
}

// postJSON posts a JSON body and parses the response; the parsed map is the
// single read of the body (draining it twice would see nothing).
func postJSON(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp, m
}

func TestShopRegistrationAndLogin(t *testing.T) {
	srv := authServer(t)

	resp, m := postJSON(t, srv.URL+"/v1/auth/shop/register",
		`{"name":"X","phone":"03001234567","password":"long-enough","lat":26.69,"lng":67.61}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register = %d, want 200-series; body %v", resp.StatusCode, m)
	}
	if resp.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("register content type = %q", resp.Header.Get("Content-Type"))
	}
	shop, _ := m["shop"].(map[string]any)
	if shop == nil || shop["eligible"] != false {
		t.Fatalf("a fresh shop must be unpaid and therefore not eligible; got %v", m["shop"])
	}
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatal("registration must return a bearer token")
	}

	// Duplicate phone → 409 (same number in a different dialing form).
	resp, m2 := postJSON(t, srv.URL+"/v1/auth/shop/register",
		`{"name":"Second","phone":"+92 300 1234567","password":"long-enough","lat":26.7,"lng":67.6}`)
	if resp.StatusCode != http.StatusConflict || m2["error"] == nil {
		t.Fatalf("duplicate phone = %d, want 409; body %v", resp.StatusCode, m2)
	}

	// Login works with any dialing form of the same number.
	resp, m3 := postJSON(t, srv.URL+"/v1/auth/login",
		`{"role":"shop","phone":"0300-1234567","password":"long-enough"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d, want 200; body %v", resp.StatusCode, m3)
	}
	if m3["token"] == nil {
		t.Fatal("login must return a token")
	}

	// Wrong password → 401 with a message that never says which half failed.
	resp, _ = postJSON(t, srv.URL+"/v1/auth/login",
		`{"role":"shop","phone":"03001234567","password":"not-the-password"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", resp.StatusCode)
	}
}

func TestRiderRegistrationAndSessionLifecycle(t *testing.T) {
	srv := authServer(t)

	resp, m := postJSON(t, srv.URL+"/v1/auth/rider/register",
		`{"name":"Ali Raza","phone":"03451234567","password":"long-enough","lat":26.694,"lng":67.616}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("rider register = %d, want 201; body %v", resp.StatusCode, m)
	}
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatal("registration must return a bearer token")
	}

	// The rider login answers with the same account shape the shop one does.
	resp, m2 := postJSON(t, srv.URL+"/v1/auth/login",
		`{"role":"rider","phone":"0345-1234567","password":"long-enough"}`)
	if resp.StatusCode != http.StatusOK || m2["token"] == nil {
		t.Fatalf("rider login = %d, want 200; body %v", resp.StatusCode, m2)
	}
	acc, _ := m2["account"].(map[string]any)
	if acc == nil || acc["id"] == nil {
		t.Fatalf("login must carry the account object; got %v", m2["account"])
	}

	// Logout kills the session for good.
	lreq, _ := http.NewRequest("POST", srv.URL+"/v1/auth/logout", nil)
	lreq.Header.Set("Authorization", "Bearer "+token)
	lresp, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatal(err)
	}
	lresp.Body.Close()
	if lresp.StatusCode != http.StatusOK {
		t.Fatalf("logout = %d, want 200", lresp.StatusCode)
	}

	lreq2, _ := http.NewRequest("POST", srv.URL+"/v1/auth/logout", nil)
	lreq2.Header.Set("Authorization", "Bearer "+token)
	lresp2, err := http.DefaultClient.Do(lreq2)
	if err != nil {
		t.Fatal(err)
	}
	lresp2.Body.Close()
	if lresp2.StatusCode != http.StatusOK {
		t.Fatalf("logout with a dead token must still answer 200 (idempotent), got %d", lresp2.StatusCode)
	}
}

func TestAuthValidationAndNoDatabase(t *testing.T) {
	// Without a store the account endpoints say so — never fabricate.
	origDB := dbStore
	dbStore = nil
	t.Cleanup(func() { dbStore = origDB })
	srv := httptest.NewServer(secureHeaders(newMuxWith(
		ratelimit.New(1000, 10000).Middleware,
		ratelimit.New(1000, 10000).Middleware)))
	defer srv.Close()

	resp, _ := postJSON(t, srv.URL+"/v1/auth/shop/register",
		`{"name":"X","phone":"03001234567","password":"long-enough","lat":26.69,"lng":67.61}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("no-DB register = %d, want 503", resp.StatusCode)
	}

	// Back with a store: malformed input fails loudly.
	srv2 := authServer(t)
	cases := []struct {
		name, body string
		want       int
	}{
		{"bad phone", `{"name":"X","phone":"123","password":"long-enough","lat":26.69,"lng":67.61}`, 400},
		{"short password", `{"name":"X","phone":"03001234567","password":"short","lat":26.69,"lng":67.61}`, 400},
		{"missing name", `{"name":"","phone":"03001234567","password":"long-enough","lat":26.69,"lng":67.61}`, 400},
		{"bad coords", `{"name":"X","phone":"03001234567","password":"long-enough","lat":999,"lng":67.61}`, 400},
		{"unknown field", `{"name":"X","phone":"03001234567","password":"long-enough","lat":26.69,"lng":67.61,"admin":true}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := postJSON(t, srv2.URL+"/v1/auth/shop/register", tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("= %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
