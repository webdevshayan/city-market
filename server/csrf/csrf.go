// Package csrf implements double-submit-cookie CSRF protection: a random
// token is stored in a hardened cookie and must be echoed back on every
// state-changing request — via the X-CSRF-Token header (how htmx sends it,
// configured once on <body> with hx-headers) or the _csrf form field.
// Comparison is constant-time.
package csrf

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

const (
	cookieName = "cm_csrf"
	headerName = "X-CSRF-Token"
	formField  = "_csrf"
)

// SecureCookie marks the CSRF cookie Secure. TLS terminates at the edge on
// Vercel, so r.TLS is not a reliable signal there; the server sets this from
// BASE_URL before serving.
var SecureCookie = false

type ctxKey struct{}

// randomToken returns 32 bytes of CSPRNG entropy hex-encoded.
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("csrf: entropy unavailable: " + err.Error()) // CSPRNG failure is unrecoverable
	}
	return hex.EncodeToString(b)
}

func setCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true, // JS never needs it: the server embeds the token in the DOM
		SameSite: http.SameSiteStrictMode,
		Secure:   SecureCookie,
	})
}

func tokenFromCookie(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

// FromRequest exposes the token templates should embed (hx-headers). The
// middleware stores it in the request context on safe requests so the very
// first visit already renders a usable token alongside the fresh cookie.
func FromRequest(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKey{}).(string); ok {
		return v
	}
	t, _ := tokenFromCookie(r)
	return t
}

// Middleware issues the cookie on safe requests and validates the echoed
// token on state-changing ones.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			token, ok := tokenFromCookie(r)
			if !ok {
				token = randomToken()
				setCookie(w, token)
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, token)))
		default:
			cookieToken, ok := tokenFromCookie(r)
			if !ok {
				forbidden(w)
				return
			}
			supplied := r.Header.Get(headerName)
			if supplied == "" {
				supplied = r.PostFormValue(formField)
			}
			if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(supplied)) != 1 {
				forbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

func forbidden(w http.ResponseWriter) {
	http.Error(w, "Forbidden — missing or invalid CSRF token", http.StatusForbidden)
}
