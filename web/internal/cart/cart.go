// Package cart implements the visitor's cart as ONE HMAC-signed cookie:
// stateless (no session table, no database), serverless-safe (any instance
// can serve any request), tamper-proof (a value without a valid signature
// is discarded as if the cart were empty), and capped (MaxItems distinct
// products, MaxQty per line).
package cart

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

const (
	// CookieName is the cart cookie's name.
	CookieName = "cm_cart"

	// MaxItems caps distinct products; MaxQty caps one line's quantity.
	MaxItems = 50
	MaxQty   = 99
)

// Cart signs and reads cart cookies.
type Cart struct {
	secret []byte
	secure bool
}

// New builds a cart signer. An empty secret generates an ephemeral CSPRNG
// key: safe by default (nothing forgeable is trusted) and honest — carts
// reset when the process does. Set CART_SECRET in production to keep carts
// across deploys.
func New(secret string, secure bool) *Cart {
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic("cart: entropy unavailable: " + err.Error()) // CSPRNG failure is unrecoverable
		}
		secret = hex.EncodeToString(b)
	}
	return &Cart{secret: []byte(secret), secure: secure}
}

type line struct {
	id  string
	qty int
}

// parse verifies the cookie's signature and decodes the payload. Any
// malformation — bad signature, bad encoding, out-of-range quantities —
// yields an empty cart: never an error surface, never a panic.
func (c *Cart) parse(r *http.Request) []line {
	ck, err := r.Cookie(CookieName)
	if err != nil || ck.Value == "" {
		return nil
	}
	payload, sig, found := strings.Cut(ck.Value, ".")
	if !found {
		return nil
	}
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	if !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}
	var out []line
	for _, part := range strings.Split(string(raw), ",") {
		if part == "" {
			continue
		}
		id, q, found := strings.Cut(part, ":")
		if !found || id == "" {
			return nil // malformed payload → discard entirely, not partially
		}
		qty, err := strconv.Atoi(q)
		if err != nil || qty < 1 || qty > MaxQty || len(id) > 64 ||
			strings.ContainsAny(id, ":,") {
			return nil
		}
		out = append(out, line{id: id, qty: qty})
	}
	if len(out) > MaxItems {
		return nil
	}
	return out
}

// serialize encodes and signs the cart value.
func (c *Cart) serialize(lines []line) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = l.id + ":" + strconv.Itoa(l.qty)
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, ",")))
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (c *Cart) setCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   c.secure,
	})
}

// Items returns the cart contents (product id → quantity); an absent or
// tampered cookie reads as empty.
func (c *Cart) Items(r *http.Request) map[string]int {
	out := map[string]int{}
	for _, l := range c.parse(r) {
		out[l.id] = l.qty
	}
	return out
}

// Count returns the total number of items across all lines.
func (c *Cart) Count(r *http.Request) int {
	total := 0
	for _, l := range c.parse(r) {
		total += l.qty
	}
	return total
}

// Add increments the quantity of productID (creating the line if needed)
// and rewrites the signed cookie. It reports false when the ID is invalid
// or the cart is at capacity; the cookie is left untouched in those cases.
func (c *Cart) Add(w http.ResponseWriter, r *http.Request, productID string) (int, bool) {
	if productID == "" || len(productID) > 64 || strings.ContainsAny(productID, ":,") {
		return 0, false
	}
	lines := c.parse(r)
	for i := range lines {
		if lines[i].id == productID {
			if lines[i].qty >= MaxQty {
				return 0, false
			}
			lines[i].qty++
			c.setCookie(w, c.serialize(lines))
			return c.total(lines), true
		}
	}
	if len(lines) >= MaxItems {
		return 0, false
	}
	lines = append(lines, line{id: productID, qty: 1})
	c.setCookie(w, c.serialize(lines))
	return c.total(lines), true
}

func (c *Cart) total(lines []line) int {
	total := 0
	for _, l := range lines {
		total += l.qty
	}
	return total
}
