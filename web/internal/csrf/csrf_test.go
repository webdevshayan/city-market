package csrf

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFProtection(t *testing.T) {
	p := Protector{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("token=" + p.FromRequest(r)))
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(p.Middleware(next))
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	base, _ := url.Parse(srv.URL)

	// State-changing POST with no cookie at all → 403.
	resp, err := client.Post(srv.URL+"/cart/add", "application/x-www-form-urlencoded",
		strings.NewReader("product_id=beef-1kg"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without cookie = %d, want 403", resp.StatusCode)
	}

	// Safe GET issues the hardened cookie and exposes the token to templates.
	resp, err = client.Get(srv.URL + "/shop")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var token string
	for _, c := range client.Jar.Cookies(base) {
		if c.Name == "cm_csrf" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("GET must issue the cm_csrf cookie")
	}
	if !strings.Contains(string(body), "token="+token) {
		t.Fatal("template-facing token must match the cookie")
	}
	if !strings.Contains(resp.Header.Get("Set-Cookie"), "HttpOnly") {
		t.Fatal("CSRF cookie must be HttpOnly")
	}

	// Correct header → through; correct form field → through; wrong → 403.
	for _, tc := range []struct {
		name, mode string
		want       int
	}{
		{"header token", "header", http.StatusOK},
		{"form-field fallback", "form", http.StatusOK},
		{"wrong token", "wrong", http.StatusForbidden},
	} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/cart/add",
			strings.NewReader("product_id=beef-1kg&_csrf="+token))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		switch tc.mode {
		case "header":
			req.Header.Set("X-CSRF-Token", token)
		case "wrong":
			req.Header.Set("X-CSRF-Token", "nope")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s: POST = %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}
