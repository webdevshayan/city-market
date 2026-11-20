package main

import (
	"net/http"
	"net/http/httptest"
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
