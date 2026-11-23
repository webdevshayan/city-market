// Package auth implements the marketplace's account layer: bcrypt password
// hashing, bearer-token sessions (only a SHA-256 of the token is stored),
// role-checked middleware for shop and rider endpoints, and the +92 phone
// normalization Pakistan's mobile numbers get everywhere in this codebase.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Roles. A shop token does not open rider endpoints and vice versa —
// middleware enforces the exact role.
const (
	RoleShop    = "shop"
	RoleRider   = "rider"
	RoleCitizen = "citizen"
)

// SessionTTL is how long a bearer token stays valid (30 days); logout
// deletes it earlier.
const SessionTTL = 30 * 24 * time.Hour

// SessionStore is the storage surface the middleware and login flow need —
// *store.Store implements it.
type SessionStore interface {
	CreateSession(tokenHash, role, subject string, ttl time.Duration) error
	SessionSubject(tokenHash string) (role, subject string, err error)
	DeleteSession(tokenHash string) error
}

// Sentinel errors the handlers map to status codes.
var (
	ErrBadPassword    = errors.New("auth: password must be 8–72 characters")
	ErrBadPhone       = errors.New("auth: phone must be a Pakistani mobile number (03xx… or +92xx…)")
	ErrBadCredentials = errors.New("auth: wrong phone or password")
)

var phoneShape = regexp.MustCompile(`^92\d{10}$`)

// NormalizePhone canonicalizes a Pakistani mobile number to 92XXXXXXXXXX:
// +92 300 1234567, 0300-1234567, 923001234567 and 3001234567 with a 92
// prefix all collapse to the same value. Anything else is ErrBadPhone.
func NormalizePhone(raw string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	switch {
	case strings.HasPrefix(digits, "0") && len(digits) == 11:
		digits = "92" + digits[1:]
	case strings.HasPrefix(digits, "92") && len(digits) == 12:
	case len(digits) == 10 && strings.HasPrefix(digits, "3"):
		digits = "92" + digits
	default:
		return "", ErrBadPhone
	}
	if !phoneShape.MatchString(digits) {
		return "", ErrBadPhone
	}
	return digits, nil
}

// HashPassword produces the bcrypt digest a store row keeps.
func HashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 72 {
		return "", ErrBadPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", fmt.Errorf("auth: hash: %w", err)
	}
	return string(h), nil
}

// CheckPassword compares a plaintext candidate against a stored digest.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewToken returns (raw, hash): the raw token goes to the client once, the
// SHA-256 hex goes into the sessions table.
func NewToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken is the stored representation of a bearer token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type ctxKey struct{}

// Middleware resolves the Bearer token, requires the exact role, and puts
// the account ID into the request context. Anything wrong is 401 (unknown
// or expired) or 403 (valid token, wrong role).
func Middleware(sessions SessionStore, role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearer(r)
			if raw == "" {
				forbidden(w)
				return
			}
			gotRole, subject, err := sessions.SessionSubject(HashToken(raw))
			if err != nil {
				forbidden(w)
				return
			}
			if gotRole != role {
				forbidden(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, subject)))
		})
	}
}

// Subject returns the account ID the middleware resolved for this request.
func Subject(r *http.Request) string {
	v, _ := r.Context().Value(ctxKey{}).(string)
	return v
}

// MonthShape is the "YYYY-MM" format every paid_through value uses.
var monthShape = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// CurrentMonth renders the UTC month containing now as "YYYY-MM".
func CurrentMonth(now time.Time) string {
	return now.Format("2006-01")
}

// PaidThroughCovers reports whether a shop's paid_through month still
// covers now (lexicographic order is month order for zero-padded YYYY-MM).
// An empty value covers nothing — a never-paid shop is never eligible.
func PaidThroughCovers(paidThrough string, now time.Time) bool {
	if !monthShape.MatchString(paidThrough) {
		return false
	}
	return paidThrough >= CurrentMonth(now)
}

// BearerOf extracts the raw bearer token from an Authorization header,
// empty when absent — the shared parser for required and optional auth.
func BearerOf(r *http.Request) string {
	return bearer(r)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

func forbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"sign in required"}` + "\n"))
}
