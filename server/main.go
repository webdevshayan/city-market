// Command server is City Market's API. It is a plain net/http server so it
// runs unchanged locally (go run .) and on Vercel's Go runtime, which runs a
// standard Go server from the project root and expects it to listen on the
// PORT environment variable.
//
// The public web app is a separate standalone project on its own domain
// (see web/) — it reads the catalog from GET /v1/products and places
// orders through POST /v1/orders. This deployment is API-only: JSON in,
// JSON out, HTML nowhere.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/webdevshayan/city-market/server/catalog"
	"github.com/webdevshayan/city-market/server/finance"
	"github.com/webdevshayan/city-market/server/match"
	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/ratelimit"
	"github.com/webdevshayan/city-market/server/store"
)

// cat is the active catalog provider, places the dispatch-candidate source;
// dbStore is set when DATABASE_URL is wired. Defaults are in-memory (seed
// catalog, fixture places) so tests and local runs work with zero
// configuration; main() switches to the Turso-backed implementations when
// the env var is present.
var (
	cat      catalog.Provider = catalog.NewSeed()
	places   placesSource     = seedPlaces{}
	orderSvc *orders.Service  // nil until DATABASE_URL is wired — orders are durable by nature
	dbStore  *store.Store
)

// placesSource supplies dispatch candidates to the matching API. *store.Store
// implements it when Turso is wired; seedPlaces adapts the launch fixtures
// otherwise.
type placesSource interface {
	Shops() ([]match.Place, error)
	Riders() ([]match.Place, error)
}

// seedPlaces adapts the launch fixtures (the no-database fallback).
type seedPlaces struct{}

func (seedPlaces) Shops() ([]match.Place, error) {
	src := store.SeedShops()
	out := make([]match.Place, len(src))
	for i, s := range src {
		out[i] = match.Place{ID: s.ID, Name: s.Name, Pos: match.Point{Lat: s.Lat, Lng: s.Lng}, Active: s.Active}
	}
	return out, nil
}

func (seedPlaces) Riders() ([]match.Place, error) {
	src := store.SeedRiders()
	out := make([]match.Place, len(src))
	for i, s := range src {
		out[i] = match.Place{ID: s.ID, Name: s.Name, Pos: match.Point{Lat: s.Lat, Lng: s.Lng}, Active: s.Available}
	}
	return out, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	wireDatabase()
	addr := ":" + port
	log.Printf("city-market API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, secureHeaders(newMux())))
}

// wireDatabase switches the catalog, carts and dispatch sources to Turso
// when DATABASE_URL is set. A failure to reach the database degrades to the
// seed fixtures with a loud log line — the site stays up rather than dying
// on boot, and operations sees the warning.
func wireDatabase() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Printf("DATABASE_URL not set — serving seed fixtures")
		return
	}
	st, err := store.Open(dsn)
	if err != nil {
		log.Printf("database open failed (%v) — serving seed fixtures", err)
		return
	}
	fail := func(stage string, err error) {
		_ = st.Close()
		log.Printf("database %s failed (%v) — serving seed fixtures", stage, err)
	}
	if err := st.EnsureSchema(); err != nil {
		fail("schema", err)
		return
	}
	if err := st.SeedIfEmpty(store.SeedShops(), store.SeedRiders(), store.SeedProducts()); err != nil {
		fail("seed", err)
		return
	}
	p, err := catalog.NewDB(st)
	if err != nil {
		fail("catalog load", err)
		return
	}
	cat = p
	dbStore = st
	places = st
	orderSvc = orders.NewService(cat, places, st.DB())
	log.Printf("catalog, orders and dispatch data served from Turso")
}

func newMux() *http.ServeMux {
	return newMuxWith(ratelimit.New(10, 20).Middleware, ratelimit.New(2, 8).Middleware)
}

// newMuxWith wires the route table. The two injectable middlewares
// rate-limit the public read API (finance + match + catalog queries at 10/s
// burst 20 per IP) and state-changing writes — orders and admin upserts (2/s
// burst 8) — injectable so tests can use tiny bursts deterministically. On
// Vercel the limiter is per instance; a durable cross-instance limit needs
// shared state.
func newMuxWith(apiLimit, cartLimit func(http.Handler) http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", handleAPIIndex)
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("GET /v1/finance/ujrah", apiLimit(http.HandlerFunc(handleUjrah)))
	mux.Handle("GET /v1/finance/rider", apiLimit(http.HandlerFunc(handleRiderPayout)))
	mux.Handle("GET /v1/finance/checkout-fee", apiLimit(http.HandlerFunc(handleCheckoutFee)))
	mux.Handle("GET /v1/finance/simulate-month", apiLimit(http.HandlerFunc(handleSimulateMonth)))
	mux.Handle("GET /v1/match/shop", apiLimit(http.HandlerFunc(handleMatchShop)))
	mux.Handle("GET /v1/match/rider", apiLimit(http.HandlerFunc(handleMatchRider)))
	mux.Handle("GET /v1/products", apiLimit(http.HandlerFunc(handlePublicProducts)))
	mux.Handle("POST /v1/orders", cartLimit(http.HandlerFunc(handlePlaceOrder)))
	mux.Handle("GET /v1/orders/{id}", apiLimit(http.HandlerFunc(handleGetOrder)))
	mux.Handle("GET /v1/shop/orders", apiLimit(http.HandlerFunc(handleShopOrders)))
	mux.Handle("GET /v1/rider/orders", apiLimit(http.HandlerFunc(handleRiderOrders)))
	mux.Handle("POST /v1/shop/orders/{id}/confirm", cartLimit(http.HandlerFunc(handleOrderTransition(orders.StatusConfirmed))))
	mux.Handle("POST /v1/rider/orders/{id}/pick-up", cartLimit(http.HandlerFunc(handleOrderTransition(orders.StatusPickedUp))))
	mux.Handle("POST /v1/rider/orders/{id}/deliver", cartLimit(http.HandlerFunc(handleOrderTransition(orders.StatusDelivered))))
	// Admin console: rate limit OUTSIDE auth so token guessing is throttled.
	mux.Handle("GET /v1/admin/shops", apiLimit(adminAuth(http.HandlerFunc(handleAdminListShops))))
	mux.Handle("GET /v1/admin/riders", apiLimit(adminAuth(http.HandlerFunc(handleAdminListRiders))))
	mux.Handle("GET /v1/admin/products", apiLimit(adminAuth(http.HandlerFunc(handleAdminListProducts))))
	mux.Handle("GET /v1/admin/commission", apiLimit(adminAuth(http.HandlerFunc(handleAdminCommission))))
	mux.Handle("GET /v1/admin/stats", apiLimit(adminAuth(http.HandlerFunc(handleAdminStats))))
	mux.Handle("PUT /v1/admin/shops/{id}", cartLimit(adminAuth(http.HandlerFunc(handleAdminUpsertShop))))
	mux.Handle("PUT /v1/admin/riders/{id}", cartLimit(adminAuth(http.HandlerFunc(handleAdminUpsertRider))))
	mux.Handle("PUT /v1/admin/products/{id}", cartLimit(adminAuth(http.HandlerFunc(handleAdminUpsertProduct))))
	// Catch-all: anything unmatched gets a JSON 404 — this deployment serves
	// no HTML pages. Go's mux routes the most specific pattern first, so this
	// never shadows the routes above.
	mux.HandleFunc("GET /", apiNotFound)
	return mux
}

// handleAPIIndex describes the API at the root path: this deployment is the
// backend only — the public web app is a separate project on its own domain
// (see web/), reading the catalog from GET /v1/products.
func handleAPIIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "city-market API",
		"note":    "the public web app is a separate deployment — see web/",
		"endpoints": []string{
			"GET /healthz",
			"GET /v1/products",
			"GET /v1/finance/{ujrah,rider,checkout-fee,simulate-month}",
			"GET /v1/match/{shop,rider}",
			"POST /v1/orders",
			"GET /v1/orders/{id}",
			"GET /v1/shop/orders?shop_id=… | GET /v1/rider/orders?rider_id=…",
			"POST /v1/shop/orders/{id}/confirm",
			"POST /v1/rider/orders/{id}/pick-up | /deliver",
			"GET|PUT /v1/admin/{shops,riders,products,commission,stats} (X-Admin-Token)",
		},
	})
}

// apiNotFound answers unmatched paths with JSON — this deployment serves no
// HTML pages.
func apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "unknown API path — see GET / for the endpoint index")
}

// secureHeaders is the security baseline (Phase 1 review groundwork):
// every response carries a restrictive CSP with no inline script allowance,
// plus the standard no-sniff / frame / referrer guards.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}

// parsePoint reads required lat/lng query params with range validation.
func parsePoint(r *http.Request) (match.Point, error) {
	latRaw := r.URL.Query().Get("lat")
	lngRaw := r.URL.Query().Get("lng")
	if latRaw == "" || lngRaw == "" {
		return match.Point{}, errors.New("missing required query parameters: lat, lng")
	}
	lat, errLat := strconv.ParseFloat(latRaw, 64)
	lng, errLng := strconv.ParseFloat(lngRaw, 64)
	if errLat != nil || errLng != nil {
		return match.Point{}, errors.New("lat and lng must be decimal degrees")
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return match.Point{}, errors.New("lat must be within [-90, 90] and lng within [-180, 180]")
	}
	return match.Point{Lat: lat, Lng: lng}, nil
}

// handleMatchShop answers "which shop fulfils this order?" — the nearest
// active shop to the customer, optionally restricted to the shop stocking a
// given product. Dispatch reads the Turso store when wired, seed fixtures
// otherwise; ties break deterministically (match.Nearest).
func handleMatchShop(w http.ResponseWriter, r *http.Request) {
	origin, err := parsePoint(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	shops, err := places.Shops()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "dispatch source unavailable")
		return
	}
	if pid := r.URL.Query().Get("product"); pid != "" {
		p, ok := cat.ByID(pid)
		if !ok {
			writeErr(w, http.StatusNotFound, "unknown product")
			return
		}
		var stocking []match.Place
		for _, s := range shops {
			if s.ID == p.ShopID {
				stocking = append(stocking, s)
			}
		}
		shops = stocking
	}
	best, dist, err := match.Nearest(origin, shops)
	if errors.Is(err, match.ErrNoCandidate) {
		writeErr(w, http.StatusNotFound, "no shop available near you")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":              best.ID,
		"name":            best.Name,
		"lat":             best.Pos.Lat,
		"lng":             best.Pos.Lng,
		"distance_meters": math.Round(dist),
	})
}

// handleMatchRider answers "which rider takes this delivery?" — the nearest
// available rider to the pickup point.
func handleMatchRider(w http.ResponseWriter, r *http.Request) {
	origin, err := parsePoint(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	riders, err := places.Riders()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "dispatch source unavailable")
		return
	}
	best, dist, err := match.Nearest(origin, riders)
	if errors.Is(err, match.ErrNoCandidate) {
		writeErr(w, http.StatusNotFound, "no rider available right now")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":              best.ID,
		"name":            best.Name,
		"lat":             best.Pos.Lat,
		"lng":             best.Pos.Lng,
		"distance_meters": math.Round(dist),
	})
}

// placeOrderBody is the citizen app's order request. Unknown fields are
// rejected so silent client/server drift fails loudly instead of half-working.
type placeOrderBody struct {
	Items []struct {
		ProductID string `json:"product_id"`
		Qty       int64  `json:"qty"`
	} `json:"items"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// handlePlaceOrder is the citizen checkout: priced with the finance engine,
// assigned shop + rider with the match engine, persisted in Turso. The cart
// write bucket guards it too — both are state-changing citizen posts.
func handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	var body placeOrderBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	items := make([]orders.Item, 0, len(body.Items))
	for _, it := range body.Items {
		items = append(items, orders.Item{ProductID: it.ProductID, Qty: it.Qty})
	}
	o, err := orderSvc.Place(orders.PlaceInput{Items: items, Lat: body.Lat, Lng: body.Lng})
	switch {
	case err == nil:
	case errors.Is(err, orders.ErrEmptyOrder),
		errors.Is(err, orders.ErrBadQty),
		errors.Is(err, orders.ErrBadLocation),
		errors.Is(err, orders.ErrUnknownProduct):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, orders.ErrMultiShop),
		errors.Is(err, orders.ErrNoShopOpen),
		errors.Is(err, orders.ErrNoRider):
		writeErr(w, http.StatusConflict, err.Error())
		return
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// handleGetOrder serves one order by ID (IDs carry 96 random bits, so they
// are unguessable and this stays unauthenticated in the demo).
func handleGetOrder(w http.ResponseWriter, r *http.Request) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	o, err := orderSvc.ByID(r.PathValue("id"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, o)
	case errors.Is(err, orders.ErrUnknownOrder):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

// handleShopOrders lists a shop's orders — the shop dashboard's queue.
func handleShopOrders(w http.ResponseWriter, r *http.Request) {
	listOrders(w, r, r.URL.Query().Get("shop_id"), "shop_id", orderSvc.ForShop)
}

// handleRiderOrders lists a rider's assigned orders.
func handleRiderOrders(w http.ResponseWriter, r *http.Request) {
	listOrders(w, r, r.URL.Query().Get("rider_id"), "rider_id", orderSvc.ForRider)
}

func listOrders(w http.ResponseWriter, r *http.Request, id, param string, load func(string) ([]orders.Order, error)) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: "+param)
		return
	}
	list, err := load(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleOrderTransition returns the handler for one status step; legality
// and stale-state guards live in the orders engine, and a skipped step or a
// repeat is a 409, never a silent state change.
func handleOrderTransition(to orders.Status) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if orderSvc == nil {
			writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
			return
		}
		o, err := orderSvc.Transition(r.PathValue("id"), to)
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, o)
		case errors.Is(err, orders.ErrUnknownOrder):
			writeErr(w, http.StatusNotFound, err.Error())
		case errors.Is(err, orders.ErrInvalidTransition):
			writeErr(w, http.StatusConflict, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
	}
}

// adminAuth guards the admin console API. An unset ADMIN_TOKEN disables the
// console entirely — default-deny; a wrong token gets 401 via a
// constant-time compare. Rate limiting wraps this, not the reverse, so
// token guessing is throttled.
func adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := os.Getenv("ADMIN_TOKEN")
		if token == "" {
			writeErr(w, http.StatusServiceUnavailable, "admin console disabled — set ADMIN_TOKEN")
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Admin-Token")), []byte(token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminWrite gates the admin write paths: admin changes are durable by
// nature, so without a database they are refused, not faked.
func adminWrite(w http.ResponseWriter) bool {
	if dbStore == nil || !dbStore.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "admin writes require Turso — set DATABASE_URL")
		return false
	}
	return true
}

func handleAdminListShops(w http.ResponseWriter, r *http.Request) {
	listAdminPlaces(w, dbStore.Shops)
}

func handleAdminListRiders(w http.ResponseWriter, r *http.Request) {
	listAdminPlaces(w, dbStore.Riders)
}

func listAdminPlaces(w http.ResponseWriter, load func() ([]match.Place, error)) {
	if dbStore == nil || !dbStore.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "admin requires Turso — set DATABASE_URL")
		return
	}
	list, err := load()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func handleAdminListProducts(w http.ResponseWriter, r *http.Request) {
	if dbStore == nil || !dbStore.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "admin requires Turso — set DATABASE_URL")
		return
	}
	writeJSON(w, http.StatusOK, cat.All())
}

// handlePublicProducts serves the full catalog as JSON, unauthenticated.
// This is the read endpoint the public web app — a separate deployment on
// its own domain — builds its shop pages from, so there is exactly one
// source of truth for what is for sale.
func handlePublicProducts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, cat.All())
}

type upsertShopBody struct {
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Active bool    `json:"active"`
}

// handleAdminUpsertShop inserts or replaces a shop; dispatch reads the live
// tables, so the change takes effect on the next dispatch query.
func handleAdminUpsertShop(w http.ResponseWriter, r *http.Request) {
	if !adminWrite(w) {
		return
	}
	var b upsertShopBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	if b.Name == "" || b.Lat < -90 || b.Lat > 90 || b.Lng < -180 || b.Lng > 180 {
		writeErr(w, http.StatusBadRequest, "shop needs a name and coordinates within [-90,90] × [-180,180]")
		return
	}
	if err := dbStore.UpsertShop(store.SeedShop{ID: r.PathValue("id"), Name: b.Name, Lat: b.Lat, Lng: b.Lng, Active: b.Active}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "ok": true})
}

type upsertRiderBody struct {
	Name      string  `json:"name"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Available bool    `json:"available"`
}

func handleAdminUpsertRider(w http.ResponseWriter, r *http.Request) {
	if !adminWrite(w) {
		return
	}
	var b upsertRiderBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	if b.Name == "" || b.Lat < -90 || b.Lat > 90 || b.Lng < -180 || b.Lng > 180 {
		writeErr(w, http.StatusBadRequest, "rider needs a name and coordinates within [-90,90] × [-180,180]")
		return
	}
	if err := dbStore.UpsertRider(store.SeedRider{ID: r.PathValue("id"), Name: b.Name, Lat: b.Lat, Lng: b.Lng, Available: b.Available}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "ok": true})
}

type upsertProductBody struct {
	Name       string `json:"name"`
	Category   string `json:"category"`
	Brand      string `json:"brand"`
	Size       string `json:"size"`
	PriceMinor int64  `json:"price_minor"`
	Image      string `json:"image"`
	ShopID     string `json:"shop_id"`
	Stock      int64  `json:"stock"`
}

// handleAdminUpsertProduct writes a catalog row and reloads the catalog
// snapshot (and the order service that reads it) so the change is live
// without a restart.
func handleAdminUpsertProduct(w http.ResponseWriter, r *http.Request) {
	if !adminWrite(w) {
		return
	}
	var b upsertProductBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	if err := dbStore.UpsertProduct(store.SeedProduct{
		ID: r.PathValue("id"), Name: b.Name, Category: b.Category, Brand: b.Brand,
		Size: b.Size, PriceMinor: b.PriceMinor, Image: b.Image, ShopID: b.ShopID,
		Stock: b.Stock,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := catalog.NewDB(dbStore)
	if err != nil {
		log.Printf("admin product upsert: catalog reload failed (%v) — restart required", err)
	} else {
		cat = p
		orderSvc = orders.NewService(cat, places, dbStore.DB())
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "ok": true})
}

// handleAdminCommission renders the fixed Ujrah schedule for a shop's
// monthly orders and sales (settlement calculator for the console). The
// schedule itself is fixed by the product specification — it is computed,
// never stored or edited.
func handleAdminCommission(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	monthlyOrders, err1 := strconv.ParseInt(q.Get("orders"), 10, 64)
	salesMinor, err2 := strconv.ParseInt(q.Get("sales"), 10, 64)
	if err1 != nil || err2 != nil {
		writeErr(w, http.StatusBadRequest, "orders and sales must be integers (sales in minor units)")
		return
	}
	commission, rateBps, capped, err := finance.CommissionMinor(monthlyOrders, salesMinor)
	switch {
	case err == nil:
	case errors.Is(err, finance.ErrNegative), errors.Is(err, finance.ErrOutOfBounds):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"monthly_orders":      monthlyOrders,
		"sales_minor":         salesMinor,
		"commission_minor":    commission,
		"commission_rate_bps": rateBps,
		"commission_capped":   capped,
	})
}

// handleAdminStats serves the admin console's stats view: revenue (GMV and
// the platform's Ujrah), rider payouts, and who is working and earning.
// The math lives in the orders service (orders.Service.Stats) — the
// handler only guards the nil store and serializes.
func handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	snap, err := orderSvc.Stats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// decodeStrict decodes a JSON body with a size cap and unknown-field
// rejection; on failure it has already written the 400.
func decodeStrict(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return err
	}
	return nil
}

// handleHealth is the API health check. The database field names the active
// store — "turso" once DATABASE_URL is wired, "seed" while serving fixtures
// (docs/DEPLOY.md lists the env vars that flip it).
func handleHealth(w http.ResponseWriter, r *http.Request) {
	db := "seed"
	if dbStore != nil {
		db = "turso"
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"service":  "city-market-server",
		"database": db,
	})
}

func handleUjrah(w http.ResponseWriter, r *http.Request) {
	orders, ok, err := queryInt64(r, "orders")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "orders must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: orders")
		return
	}
	salesMinor, ok, err := queryInt64(r, "sales_minor")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "sales_minor must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: sales_minor")
		return
	}

	commissionMinor, rateBps, capped, err := finance.CommissionMinor(orders, salesMinor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orders":            orders,
		"sales_minor":       salesMinor,
		"rate_bps":          rateBps,
		"rate_percent":      rateBps / 100,
		"commission_minor":  commissionMinor,
		"commission_major":  commissionMinor / finance.MinorPerUnit,
		"commission_capped": capped,
	})
}

func handleRiderPayout(w http.ResponseWriter, r *http.Request) {
	deliveries, ok, err := queryInt64(r, "deliveries")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "deliveries must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: deliveries")
		return
	}
	km, ok, err := queryInt64(r, "km")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "km must be an integer")
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "missing required query parameter: km")
		return
	}

	payoutMinor, baseMinor, distanceMinor, err := finance.RiderPayoutMinor(deliveries, km)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deliveries":     deliveries,
		"km":             km,
		"base_minor":     baseMinor,
		"distance_minor": distanceMinor,
		"payout_minor":   payoutMinor,
		"payout_major":   payoutMinor / finance.MinorPerUnit,
	})
}

func handleCheckoutFee(w http.ResponseWriter, r *http.Request) {
	feeMinor := finance.CustomerDeliveryFeeMinor()
	writeJSON(w, http.StatusOK, map[string]any{
		"fee_minor": feeMinor,
		"fee_major": feeMinor / finance.MinorPerUnit,
	})
}

func handleSimulateMonth(w http.ResponseWriter, r *http.Request) {
	// All parameters are optional and default to zero so the endpoint can be
	// probed easily; production callers pass the full month's activity.
	orders, _, err := queryInt64(r, "orders")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "orders must be an integer")
		return
	}
	salesMinor, _, err := queryInt64(r, "sales_minor")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "sales_minor must be an integer")
		return
	}
	deliveries, _, err := queryInt64(r, "deliveries")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "deliveries must be an integer")
		return
	}
	km, _, err := queryInt64(r, "km")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "km must be an integer")
		return
	}

	summary, err := finance.SimulateMonth(orders, salesMinor, deliveries, km)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func queryInt64(r *http.Request, key string) (int64, bool, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, true, err
	}
	return v, true, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
