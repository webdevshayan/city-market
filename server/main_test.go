package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
