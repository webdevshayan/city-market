package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The function runs with no env configured in tests: pages must render
// (honestly empty shop, seeded 404s) rather than crash — the same contract
// a cold Vercel instance gets when an env var is missing.
func TestHandlerServesTheSite(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "city-market-web") {
		t.Fatalf("healthz via Handler = %d %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	Handler(rec2, httptest.NewRequest("GET", "/shop", nil))
	if rec2.Code != 200 {
		t.Fatalf("shop via Handler = %d", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	Handler(rec3, httptest.NewRequest("GET", "/definitely-not-here", nil))
	if rec3.Code != 404 || !strings.Contains(rec3.Body.String(), "Page not found") {
		t.Fatalf("404 via Handler = %d %s", rec3.Code, rec3.Body.String())
	}
}
