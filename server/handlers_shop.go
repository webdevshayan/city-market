// The shop owner's API — everything behind a shop bearer token: the shop's
// own profile (with its subscription state), full product CRUD on its own
// shelf, its order queue, and the accept/cancel steps of the lifecycle.
// Accepting an order is the moment goods leave the shelf: stock decrements
// inside one transaction and a short shelf fails the accept atomically.
package main

import (
	"errors"
	"net/http"

	"github.com/webdevshayan/city-market/server/auth"
	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/store"
)

// shopBearer wraps a handler with the shop-role bearer middleware.
func shopBearer(next http.HandlerFunc) http.Handler {
	return auth.Middleware(dbStore, auth.RoleShop)(next)
}

// handleShopMe serves the signed-in shop's own profile: identity, location,
// and the subscription state that decides whether the market dispatches to
// it this month.
func handleShopMe(w http.ResponseWriter, r *http.Request) {
	acc, err := dbStore.ShopByID(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "shop account missing — sign in again")
		return
	}
	writeJSON(w, http.StatusOK, shopJSON(acc))
}

// shopProducts lists the shop's own shelf: every catalog row it owns, with
// the live stock count.
func handleShopProducts(w http.ResponseWriter, r *http.Request) {
	rows, err := dbStore.Products()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	shopID := auth.Subject(r)
	out := make([]catalog.Product, 0, 4)
	for _, p := range rows {
		if p.ShopID == shopID {
			out = append(out, p)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type shopProductBody struct {
	Name       string `json:"name"`
	Category   string `json:"category"`
	Brand      string `json:"brand"`
	Size       string `json:"size"`
	PriceMinor int64  `json:"price_minor"`
	Image      string `json:"image"`
	Stock      int64  `json:"stock"`
}

// productBounds mirror the catalog's sanity limits: 1 PKR to 1,000,000 PKR
// per unit, a non-negative shelf, and non-empty listing fields.
func validateProduct(b shopProductBody) (int, string) {
	if b.Name == "" || b.Category == "" || b.Brand == "" || b.Size == "" {
		return http.StatusBadRequest, "product needs name, category, brand and size"
	}
	if b.PriceMinor < 100 || b.PriceMinor > 100_000_000 {
		return http.StatusBadRequest, "price_minor must be between 100 and 100000000 (1 PKR to 1,000,000 PKR)"
	}
	if b.Stock < 0 || b.Stock > 1_000_000 {
		return http.StatusBadRequest, "stock must be between 0 and 1000000"
	}
	return 0, ""
}

// handleShopCreateProduct adds a listing to the signed-in shop's shelf and
// reloads the catalog snapshot so the marketplace sees it immediately.
func handleShopCreateProduct(w http.ResponseWriter, r *http.Request) {
	var b shopProductBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	if status, msg := validateProduct(b); status != 0 {
		writeErr(w, status, msg)
		return
	}
	id, err := newAccountID("p-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	shopID := auth.Subject(r)
	if err := dbStore.UpsertProduct(store.SeedProduct{
		ID: id, Name: b.Name, Category: b.Category, Brand: b.Brand,
		Size: b.Size, PriceMinor: b.PriceMinor, Image: b.Image,
		ShopID: shopID, Stock: b.Stock,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reloadCatalog()
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "ok": true})
}

// handleShopUpdateProduct rewrites one of the shop's own listings. The
// ownership check is the shop_id on the row — another shop's product is a
// 404, which leaks nothing.
func handleShopUpdateProduct(w http.ResponseWriter, r *http.Request) {
	var b shopProductBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	shopID := auth.Subject(r)
	id := r.PathValue("id")
	rows, err := dbStore.Products()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	owned := false
	for _, p := range rows {
		if p.ID == id && p.ShopID == shopID {
			owned = true
			break
		}
	}
	if !owned {
		writeErr(w, http.StatusNotFound, "no such product on this shelf")
		return
	}
	if status, msg := validateProduct(b); status != 0 {
		writeErr(w, status, msg)
		return
	}
	if err := dbStore.UpsertProduct(store.SeedProduct{
		ID: id, Name: b.Name, Category: b.Category, Brand: b.Brand,
		Size: b.Size, PriceMinor: b.PriceMinor, Image: b.Image,
		ShopID: shopID, Stock: b.Stock,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reloadCatalog()
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "ok": true})
}

// reloadCatalog rebuilds the catalog snapshot (and the order service reading
// it) after a listing write — the same live-refresh the admin path uses.
func reloadCatalog() {
	if dbStore == nil || !dbStore.Enabled() {
		return
	}
	p, err := catalog.NewDB(dbStore)
	if err != nil {
		// Keep serving the previous snapshot rather than dropping the
		// catalog; the write itself is durable and a restart re-reads it.
		return
	}
	cat = p
	orderSvc = orders.NewService(cat, places, dbStore.DB()).WithStock(dbStore)
}

// handleShopQueue lists the signed-in shop's orders, newest first — the
// dashboard's queue.
func handleShopQueue(w http.ResponseWriter, r *http.Request) {
	list, err := orderSvc.ForShop(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleShopAccept is the confirm step with the shelf behind it: stock
// decrements first (atomically, all lines guarded), then the status
// advances. If the transition loses a race the shelf is put back — an
// accept either fully happens or fully doesn't, as far as the goods are
// concerned.
func handleShopAccept(w http.ResponseWriter, r *http.Request) {
	o, err := orderSvc.ByID(r.PathValue("id"))
	switch {
	case err == nil:
	case errors.Is(err, orders.ErrUnknownOrder):
		writeErr(w, http.StatusNotFound, err.Error())
		return
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if o.ShopID != auth.Subject(r) {
		// Another shop's order — a 404, which leaks nothing.
		writeErr(w, http.StatusNotFound, "no such order on this shop")
		return
	}
	items := map[string]int64{}
	for _, it := range o.Items {
		items[it.ProductID] += it.Qty
	}
	if err := dbStore.ReserveStock(items); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	confirmed, err := orderSvc.Transition(o.ID, orders.StatusConfirmed)
	if err != nil {
		_ = dbStore.ReleaseStock(items) // goods back on the shelf
		switch {
		case errors.Is(err, orders.ErrInvalidTransition):
			writeErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, orders.ErrUnknownOrder):
			writeErr(w, http.StatusNotFound, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, confirmed)
}

// handleShopCancel rolls a fresh order back: cancelled while placed costs
// nothing; cancelled after accept gives the goods back to the shelf.
func handleShopCancel(w http.ResponseWriter, r *http.Request) {
	o, err := orderSvc.ByID(r.PathValue("id"))
	switch {
	case err == nil:
	case errors.Is(err, orders.ErrUnknownOrder):
		writeErr(w, http.StatusNotFound, err.Error())
		return
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if o.ShopID != auth.Subject(r) {
		writeErr(w, http.StatusNotFound, "no such order on this shop")
		return
	}
	if o.Status == orders.StatusConfirmed {
		items := map[string]int64{}
		for _, it := range o.Items {
			items[it.ProductID] += it.Qty
		}
		if err := dbStore.ReleaseStock(items); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	cancelled, err := orderSvc.Transition(o.ID, orders.StatusCancelled)
	if err != nil {
		switch {
		case errors.Is(err, orders.ErrInvalidTransition):
			writeErr(w, http.StatusConflict, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, cancelled)
}
