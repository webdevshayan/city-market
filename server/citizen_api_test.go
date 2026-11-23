package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// citizenServer builds on the paid-shop fixture and registers one citizen,
// returning the server, the citizen's bearer token, and a shop token for
// accept steps.
func citizenServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	srv, shopToken, _ := shopFlowServer(t)
	resp, m := postJSON(t, srv.URL+"/v1/auth/citizen/register",
		`{"name":"Aisha Khaskheli","phone":"03111234567","password":"long-enough"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("citizen register = %d %v", resp.StatusCode, m)
	}
	token, _ := m["token"].(string)
	return srv, token, shopToken
}

func TestCitizenRegistrationAndLogin(t *testing.T) {
	srv, token, _ := citizenServer(t)

	// Duplicate phone → 409, also via a different dialing form.
	resp, m := postJSON(t, srv.URL+"/v1/auth/citizen/register",
		`{"name":"Second","phone":"+92 311 1234567","password":"long-enough"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate citizen phone = %d %v, want 409", resp.StatusCode, m)
	}

	// Login answers the same account shape as the other roles.
	resp, m = postJSON(t, srv.URL+"/v1/auth/login",
		`{"role":"citizen","phone":"0311-1234567","password":"long-enough"}`)
	if resp.StatusCode != http.StatusOK || m["token"] == nil {
		t.Fatalf("citizen login = %d %v", resp.StatusCode, m)
	}
	acc, _ := m["account"].(map[string]any)
	if acc == nil || acc["name"] != "Aisha Khaskheli" {
		t.Fatalf("login account = %v", m["account"])
	}

	// A citizen token must not open shop or rider surfaces.
	req, _ := http.NewRequest("GET", srv.URL+"/v1/shop/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("citizen token on shop API = %d, want 401", resp2.StatusCode)
	}

	// History starts empty.
	code, _, raw := bearerDo(t, srv, token, "GET", "/v1/citizen/orders", "")
	if code != http.StatusOK || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("empty history = %d %s, want []", code, raw)
	}
}

func TestCitizenOrdersAttachAndHistoryListsThem(t *testing.T) {
	srv, token, shopToken := citizenServer(t)

	// A signed-in citizen places an order: the bearer attaches the account.
	req, _ := http.NewRequest("POST", srv.URL+"/v1/orders",
		strings.NewReader(`{"items":[{"product_id":"beef-1kg","qty":1}],"lat":26.6931,"lng":67.6156,"address_note":"Near the water tank"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("attached placement = %d; body %s", resp.StatusCode, raw)
	}
	var placed struct {
		ID        string `json:"id"`
		CitizenID string `json:"citizen_id"`
	}
	if err := json.Unmarshal(raw, &placed); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(placed.CitizenID, "ct-") {
		t.Fatalf("citizen_id = %q, want the account attached", placed.CitizenID)
	}

	// History lists orders in every status — no accept step needed here.
	// (The product belongs to the seeded s-central, which has no login.)
	code, _, raw := bearerDo(t, srv, token, "GET", "/v1/citizen/orders", "")
	if code != http.StatusOK {
		t.Fatalf("history = %d", code)
	}
	var history []map[string]any
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0]["id"] != placed.ID {
		t.Fatalf("history = %v, want the attached order", history)
	}

	// A guest order (no header) belongs to nobody.
	resp2, _ := postOrder(t, srv.URL, `{"items":[{"product_id":"beef-1kg","qty":1}],"lat":26.6931,"lng":67.6156}`)
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("guest placement = %d", resp2.StatusCode)
	}
	code, _, raw = bearerDo(t, srv, token, "GET", "/v1/citizen/orders", "")
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history after a guest order = %d entries, want only the attached one", len(history))
	}

	// A broken token on placement is a 401, never a silent guest order.
	bad, _ := http.NewRequest("POST", srv.URL+"/v1/orders",
		strings.NewReader(`{"items":[{"product_id":"beef-1kg","qty":1}],"lat":26.69,"lng":67.61}`))
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("Authorization", "Bearer garbage-token")
	resp3, err := http.DefaultClient.Do(bad)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("broken token placement = %d, want 401", resp3.StatusCode)
	}

	// A shop token is not a citizen token either.
	code, _, _ = bearerDo(t, srv, shopToken, "GET", "/v1/citizen/orders", "")
	if code != http.StatusUnauthorized {
		t.Fatalf("shop token on citizen history = %d, want 401", code)
	}
}
