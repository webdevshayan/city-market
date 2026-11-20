package csrf

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareIssuesCookieOnGET(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromRequest(r) == "" {
			t.Error("handler must see the fresh token via the request context")
		}
	})
	srv := httptest.NewServer(Middleware(next))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "cm_csrf" {
			found = true
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				t.Errorf("cookie hardening missing: %+v", c)
			}
			if len(c.Value) < 32 {
				t.Errorf("token too short: %q", c.Value)
			}
		}
	}
	if !found {
		t.Fatal("GET did not issue the cm_csrf cookie")
	}
}

func TestMiddlewareRejectsMissingTokenOnPOST(t *testing.T) {
	srv := httptest.NewServer(Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("POST must not reach the handler without a valid token")
	})))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/x-www-form-urlencoded", strings.NewReader("a=1"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without token = %d, want 403", resp.StatusCode)
	}
}

func TestMiddlewareAcceptsMatchingHeader(t *testing.T) {
	var token string
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		token = FromRequest(r)
		next.ServeHTTP(w, r)
	})
	mux.HandleFunc("POST /action", next)
	srv := httptest.NewServer(Middleware(mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "cm_csrf" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("no token cookie issued")
	}

	req, _ := http.NewRequest("POST", srv.URL+"/action", strings.NewReader("a=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: "cm_csrf", Value: token})
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if !reached || resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST with matching header = %d (reached=%v), want 200", resp2.StatusCode, reached)
	}
}
