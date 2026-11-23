// Shop and rider accounts: registration, login, logout. Passwords are
// bcrypt digests, sessions are bearer tokens stored as SHA-256 hashes with
// a 30-day TTL, and every endpoint here is durable by nature — no database,
// no accounts (503), never a fabricated session.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/webdevshayan/city-market/server/auth"
	"github.com/webdevshayan/city-market/server/store"
)

// requireDB answers with 503 and reports whether writes can proceed.
func requireDB(w http.ResponseWriter) bool {
	if dbStore == nil || !dbStore.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "accounts require Turso — set DATABASE_URL")
		return false
	}
	return true
}

func newAccountID(prefix string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

type registerShopBody struct {
	Name     string  `json:"name"`
	Phone    string  `json:"phone"`
	Password string  `json:"password"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

// handleShopRegister creates a shop owner account. The shop lands in the
// dispatch tables immediately but starts unpaid — the admin's monthly mark
// (payment received manually) is what makes it eligible for orders.
func handleShopRegister(w http.ResponseWriter, r *http.Request) {
	if !requireDB(w) {
		return
	}
	var b registerShopBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	phone, err := auth.NormalizePhone(b.Phone)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.Name == "" || b.Lat < -90 || b.Lat > 90 || b.Lng < -180 || b.Lng > 180 {
		writeErr(w, http.StatusBadRequest, "shop needs a name and coordinates within [-90,90] × [-180,180]")
		return
	}
	if _, err := dbStore.ShopByPhone(phone); err == nil {
		writeErr(w, http.StatusConflict, "this phone number already has an account — sign in instead")
		return
	}
	id, err := newAccountID("sh-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sh := store.SeedShop{ID: id, Name: b.Name, Lat: b.Lat, Lng: b.Lng, Active: true}
	if err := dbStore.RegisterShop(sh, phone, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, _, err := issueSession(auth.RoleShop, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	acc, err := dbStore.ShopByID(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"role":  auth.RoleShop,
		"shop":  shopJSON(acc),
	})
}

type registerRiderBody struct {
	Name     string  `json:"name"`
	Phone    string  `json:"phone"`
	Password string  `json:"password"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

func handleRiderRegister(w http.ResponseWriter, r *http.Request) {
	if !requireDB(w) {
		return
	}
	var b registerRiderBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	phone, err := auth.NormalizePhone(b.Phone)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.Name == "" || b.Lat < -90 || b.Lat > 90 || b.Lng < -180 || b.Lng > 180 {
		writeErr(w, http.StatusBadRequest, "rider needs a name and coordinates within [-90,90] × [-180,180]")
		return
	}
	if _, err := dbStore.RiderByPhone(phone); err == nil {
		writeErr(w, http.StatusConflict, "this phone number already has an account — sign in instead")
		return
	}
	id, err := newAccountID("rd-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	rd := store.SeedRider{ID: id, Name: b.Name, Lat: b.Lat, Lng: b.Lng, Available: true}
	if err := dbStore.RegisterRider(rd, phone, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, _, err := issueSession(auth.RoleRider, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"role":  auth.RoleRider,
		"rider": map[string]any{"id": id, "name": b.Name, "lat": b.Lat, "lng": b.Lng, "available": true},
	})
}

type registerCitizenBody struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

// handleCitizenRegister creates a customer account. Ordering stays open to
// guests; an account exists so the market can attach order history across
// devices — nothing about checkout requires it.
func handleCitizenRegister(w http.ResponseWriter, r *http.Request) {
	if !requireDB(w) {
		return
	}
	var b registerCitizenBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	phone, err := auth.NormalizePhone(b.Phone)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if b.Name == "" {
		writeErr(w, http.StatusBadRequest, "citizen needs a name")
		return
	}
	if _, err := dbStore.CitizenByPhone(phone); err == nil {
		writeErr(w, http.StatusConflict, "this phone number already has an account — sign in instead")
		return
	}
	id, err := newAccountID("ct-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := dbStore.RegisterCitizen(id, b.Name, phone, hash); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeErr(w, http.StatusConflict, "this phone number already has an account — sign in instead")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, _, err := issueSession(auth.RoleCitizen, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   token,
		"role":    auth.RoleCitizen,
		"account": map[string]any{"id": id, "name": b.Name, "phone": phone},
	})
}

type loginBody struct {
	Role     string `json:"role"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

// handleLogin exchanges phone + password for a bearer token. The response
// is deliberately constant-shaped for both roles ("account" object) so the
// client login screen has one parser.
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if !requireDB(w) {
		return
	}
	var b loginBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	phone, err := auth.NormalizePhone(b.Phone)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	fail := func() {
		writeErr(w, http.StatusUnauthorized, auth.ErrBadCredentials.Error())
	}
	switch b.Role {
	case auth.RoleShop:
		acc, err := dbStore.ShopByPhone(phone)
		if err != nil || !auth.CheckPassword(acc.PasswordHash, b.Password) {
			fail()
			return
		}
		token, _, err := issueSession(auth.RoleShop, acc.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "role": auth.RoleShop, "account": shopJSON(acc)})
	case auth.RoleRider:
		acc, err := dbStore.RiderByPhone(phone)
		if err != nil || !auth.CheckPassword(acc.PasswordHash, b.Password) {
			fail()
			return
		}
		token, _, err := issueSession(auth.RoleRider, acc.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"role":    auth.RoleRider,
			"account": map[string]any{"id": acc.ID, "name": acc.Name, "phone": acc.Phone, "available": acc.Available},
		})
	case auth.RoleCitizen:
		acc, err := dbStore.CitizenByPhone(phone)
		if err != nil || !auth.CheckPassword(acc.PasswordHash, b.Password) {
			fail()
			return
		}
		token, _, err := issueSession(auth.RoleCitizen, acc.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"role":    auth.RoleCitizen,
			"account": map[string]any{"id": acc.ID, "name": acc.Name, "phone": acc.Phone},
		})
	default:
		writeErr(w, http.StatusBadRequest, `role must be "shop", "rider" or "citizen"`)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if !requireDB(w) {
		return
	}
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if raw == "" {
		writeErr(w, http.StatusUnauthorized, "sign in required")
		return
	}
	if err := dbStore.DeleteSession(auth.HashToken(strings.TrimSpace(raw))); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func issueSession(role, subject string) (string, string, error) {
	raw, hash, err := auth.NewToken()
	if err != nil {
		return "", "", err
	}
	if err := dbStore.CreateSession(hash, role, subject, auth.SessionTTL); err != nil {
		return "", "", err
	}
	return raw, hash, nil
}

// shopJSON renders a shop account without its password digest. Eligible is
// the dispatch gate: open AND paid through the current month.
func shopJSON(a store.ShopAccount) map[string]any {
	return map[string]any{
		"id":           a.ID,
		"name":         a.Name,
		"lat":          a.Lat,
		"lng":          a.Lng,
		"active":       a.Active,
		"phone":        a.Phone,
		"paid_through": a.PaidThrough,
		"payment_note": a.PaymentNote,
		"eligible":     a.Active && auth.PaidThroughCovers(a.PaidThrough, time.Now().UTC()),
	}
}

// mapAuthErr converts the auth layer's sentinel errors to status codes.
func mapAuthErr(err error) (int, string) {
	switch {
	case err == nil:
		return 0, ""
	case errors.Is(err, auth.ErrBadPhone), errors.Is(err, auth.ErrBadPassword):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
