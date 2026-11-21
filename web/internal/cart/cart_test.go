package cart

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// carry returns a FRESH request carrying the cookies the handler set — a
// browser replaces its cart cookie, never appends a second one.
func carry(w *httptest.ResponseRecorder) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func namedCookie(value string) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: value}
}

func TestAddCountAndItems(t *testing.T) {
	c := New("test-secret", false)

	req := httptest.NewRequest("GET", "/", nil)
	if c.Count(req) != 0 {
		t.Fatal("empty cart must count 0")
	}

	w := httptest.NewRecorder()
	n, ok := c.Add(w, req, "beef-1kg")
	if !ok || n != 1 {
		t.Fatalf("first add = %d, %v; want 1, true", n, ok)
	}
	set := w.Result().Cookies()
	if len(set) != 1 || set[0].Name != CookieName {
		t.Fatalf("cookies = %v", set)
	}
	if set[0].MaxAge != 86400*30 || !set[0].HttpOnly {
		t.Fatal("cart cookie must be hardened (HttpOnly, 30-day)")
	}

	w2 := httptest.NewRecorder()
	n, ok = c.Add(w2, carry(w), "beef-1kg")
	if !ok || n != 2 {
		t.Fatalf("second add = %d, %v; want 2, true", n, ok)
	}
	if got := c.Items(carry(w2)); got["beef-1kg"] != 2 {
		t.Fatalf("items = %v, want beef-1kg:2", got)
	}
}

func TestTamperedCookieReadsEmpty(t *testing.T) {
	c := New("test-secret", false)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	if _, ok := c.Add(w, req, "beef-1kg"); !ok {
		t.Fatal("add must succeed")
	}
	value := w.Result().Cookies()[0].Value

	// Change the last signature character — the signature no longer matches.
	var tampered string
	if value[len(value)-1] == '0' {
		tampered = value[:len(value)-1] + "1"
	} else {
		tampered = value[:len(value)-1] + "0"
	}
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(namedCookie(tampered))
	if c.Count(req2) != 0 {
		t.Fatal("tampered cookie must read as an empty cart")
	}

	// A cookie signed with a different secret is equally untrusted.
	other := New("other-secret", false)
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.AddCookie(namedCookie(value))
	if other.Count(req3) != 0 {
		t.Fatal("cookie from another secret must read as empty")
	}
}

func TestCapacityLimits(t *testing.T) {
	c := New("test-secret", false)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Quantity cap: MaxQty additions succeed, the next is refused.
	for i := 0; i < MaxQty; i++ {
		w = httptest.NewRecorder()
		if _, ok := c.Add(w, req, "beef-1kg"); !ok {
			t.Fatalf("add %d refused early", i+1)
		}
		req = carry(w)
	}
	w = httptest.NewRecorder()
	if _, ok := c.Add(w, req, "beef-1kg"); ok {
		t.Fatal("add past MaxQty must be refused")
	}

	// Distinct-item cap.
	c2 := New("test-secret", false)
	req4 := httptest.NewRequest("GET", "/", nil)
	w4 := httptest.NewRecorder()
	for i := 0; i < MaxItems; i++ {
		w4 = httptest.NewRecorder()
		if _, ok := c2.Add(w4, req4, "prod-"+strconv.Itoa(i)); !ok {
			t.Fatalf("add item %d refused early", i+1)
		}
		req4 = carry(w4)
	}
	w4 = httptest.NewRecorder()
	if _, ok := c2.Add(w4, req4, "prod-"+strconv.Itoa(MaxItems)); ok {
		t.Fatal("add past MaxItems must be refused")
	}
}

func TestInvalidIDsRefused(t *testing.T) {
	c := New("test-secret", false)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	for _, id := range []string{"", "a:b", "a,b", strings.Repeat("x", 65)} {
		if _, ok := c.Add(w, req, id); ok {
			t.Fatalf("invalid id %q must be refused", id)
		}
	}
}

func TestEmptySecretIsEphemeralButWorking(t *testing.T) {
	a := New("", false)
	b := New("", false)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	if _, ok := a.Add(w, req, "beef-1kg"); !ok {
		t.Fatal("ephemeral cart must work")
	}
	if b.Count(carry(w)) != 0 {
		t.Fatal("another ephemeral instance must not trust the cookie")
	}
	if a.Count(carry(w)) != 1 {
		t.Fatal("the same instance must trust its own cookie")
	}
}
