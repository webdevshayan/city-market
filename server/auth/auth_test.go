package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	good := map[string]string{
		"03001234567":     "923001234567",
		"+92 300 1234567": "923001234567",
		"92-300-1234567":  "923001234567",
		"+923001234567":   "923001234567",
	}
	for raw, want := range good {
		got, err := NormalizePhone(raw)
		if err != nil || got != want {
			t.Fatalf("NormalizePhone(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, bad := range []string{"", "123", "0300123456", "030012345678", "abcd", "92 21 1234567"} {
		if _, err := NormalizePhone(bad); err == nil {
			t.Fatalf("NormalizePhone(%q) accepted an invalid number", bad)
		}
	}
}

func TestPasswordHashing(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("an 8-char minimum must reject short passwords")
	}
	h, err := HashPassword("good-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "good-passphrase") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(h, "wrong-passphrase") {
		t.Fatal("wrong password accepted")
	}
}

func TestPaidThroughCovers(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	cases := map[string]bool{
		"":        false,
		"2026-10": false,
		"2026-11": true,
		"2026-12": true,
		"2027-01": true,
		"2026-13": false,
		"garbage": false,
		"2026-1":  false,
		"202611":  false,
	}
	for m, want := range cases {
		if got := PaidThroughCovers(m, now); got != want {
			t.Fatalf("PaidThroughCovers(%q) = %v, want %v", m, got, want)
		}
	}
	if CurrentMonth(now) != "2026-11" {
		t.Fatalf("CurrentMonth = %q", CurrentMonth(now))
	}
}

// fakeSessions records what the flow wrote and answers reads from it.
type fakeSessions struct {
	rows map[string][2]string // tokenHash → role, subject
}

func (f *fakeSessions) CreateSession(hash, role, subject string, _ time.Duration) error {
	f.rows[hash] = [2]string{role, subject}
	return nil
}

func (f *fakeSessions) SessionSubject(hash string) (string, string, error) {
	if row, ok := f.rows[hash]; ok {
		return row[0], row[1], nil
	}
	return "", "", errors.New("no session")
}

func (f *fakeSessions) DeleteSession(hash string) error {
	delete(f.rows, hash)
	return nil
}

func TestTokenRoundTrip(t *testing.T) {
	raw, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || hash == "" || hash == raw {
		t.Fatal("token and hash must both be non-empty and distinct")
	}
	if len(hash) != 64 {
		t.Fatalf("token hash = %d hex chars, want 64 (SHA-256)", len(hash))
	}
	if HashToken(raw) != hash {
		t.Fatal("HashToken must be deterministic")
	}
}

func TestMiddlewareRoleAndSubject(t *testing.T) {
	sessions := &fakeSessions{rows: map[string][2]string{}}
	shopRaw, shopHash, _ := NewToken()
	sessions.rows[shopHash] = [2]string{RoleShop, "sh-1"}
	riderRaw, riderHash, _ := NewToken()
	sessions.rows[riderHash] = [2]string{RoleRider, "rd-1"}

	var sawSubject string
	handler := Middleware(sessions, RoleShop)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSubject = Subject(r)
		w.WriteHeader(http.StatusOK)
	}))

	do := func(token string) int {
		req := httptest.NewRequest("GET", "/", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do("x"); code != http.StatusUnauthorized {
		t.Fatalf("garbage token = %d, want 401", code)
	}
	if code := do(riderRaw); code != http.StatusUnauthorized {
		t.Fatalf("rider token on shop route = %d, want 401", code)
	}
	if code := do(shopRaw); code != http.StatusOK {
		t.Fatalf("shop token = %d, want 200", code)
	}
	if sawSubject != "sh-1" {
		t.Fatalf("subject = %q, want sh-1", sawSubject)
	}
}

// A context carry-over check: Subject outside the middleware is empty.
func TestSubjectWithoutMiddleware(t *testing.T) {
	if Subject(httptest.NewRequest("GET", "/", nil).WithContext(context.Background())) != "" {
		t.Fatal("Subject must be empty without the middleware")
	}
}
