// The rider's API: duty and position, the dispatch board (open jobs with
// real estimates before any commitment), the atomic claim, and the guarded
// pick-up/deliver steps. Everything here rides a rider bearer token —
// boards and jobs are per-rider views, never global.
package main

import (
	"errors"
	"math"
	"net/http"

	"github.com/webdevshayan/city-market/server/auth"
	"github.com/webdevshayan/city-market/server/finance"
	"github.com/webdevshayan/city-market/server/match"
	"github.com/webdevshayan/city-market/server/orders"
	"github.com/webdevshayan/city-market/server/route"
	"github.com/webdevshayan/city-market/server/store"
)

func riderBearer(next http.HandlerFunc) http.Handler {
	return auth.Middleware(dbStore, auth.RoleRider)(next)
}

// riderJSON is the signed-in rider's own account view.
func riderJSON(a store.RiderAccount) map[string]any {
	return map[string]any{
		"id":        a.ID,
		"name":      a.Name,
		"lat":       a.Lat,
		"lng":       a.Lng,
		"available": a.Available,
		"phone":     a.Phone,
	}
}

// handleRiderMe reads the signed-in rider's account.
func handleRiderMe(w http.ResponseWriter, r *http.Request) {
	acc, err := dbStore.RiderByID(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "rider account missing — sign in again")
		return
	}
	writeJSON(w, http.StatusOK, riderJSON(acc))
}

type riderMeBody struct {
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Available bool    `json:"available"`
}

// handleRiderSetMe updates duty and position in one call — the board's
// pickup distances and the on-duty stats both read this row.
func handleRiderSetMe(w http.ResponseWriter, r *http.Request) {
	var b riderMeBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	if b.Lat < -90 || b.Lat > 90 || b.Lng < -180 || b.Lng > 180 {
		writeErr(w, http.StatusBadRequest, "lat must be within [-90,90] and lng within [-180,180]")
		return
	}
	if err := dbStore.SetRiderLocation(auth.Subject(r), b.Lat, b.Lng, b.Available); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "available": b.Available})
}

// boardRow is one open job with the numbers a rider decides on: where to
// pick up, where to deliver, how far and how long, what it pays.
type boardRow struct {
	OrderID     string  `json:"order_id"`
	ShopName    string  `json:"shop_name"`
	ShopLat     float64 `json:"shop_lat"`
	ShopLng     float64 `json:"shop_lng"`
	DropLat     float64 `json:"drop_lat"`
	DropLng     float64 `json:"drop_lng"`
	AddressNote string  `json:"address_note"`
	Lines       int     `json:"lines"`
	TotalMinor  int64   `json:"total_minor"`
	PickupKm    float64 `json:"pickup_km"`
	RouteKm     float64 `json:"route_km"`
	ETAMinutes  int64   `json:"eta_minutes"`
	PayoutMinor int64   `json:"payout_minor"`
}

// handleRiderBoard lists confirmed-but-unclaimed orders with this rider's
// personalized estimates, nearest pickup first. Before selection the rider
// sees exactly where the pickup is and where the delivery goes.
func handleRiderBoard(w http.ResponseWriter, r *http.Request) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	acc, err := dbStore.RiderByID(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "rider account missing — sign in again")
		return
	}
	open, err := orderSvc.AvailableOrders()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	me := match.Point{Lat: acc.Lat, Lng: acc.Lng}
	shops, err := places.Shops()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "dispatch source unavailable")
		return
	}
	shopByID := map[string]match.Place{}
	for _, sh := range shops {
		shopByID[sh.ID] = sh
	}

	out := make([]boardRow, 0, len(open))
	for _, o := range open {
		shop, ok := shopByID[o.ShopID]
		if !ok {
			continue // the shop row vanished mid-board; skip rather than lie
		}
		drop := match.Point{Lat: o.Lat, Lng: o.Lng}
		pickupEst, err := routeEstimator.Estimate(me, shop.Pos)
		if err != nil {
			pickupEst = route.FallbackEstimate(me, shop.Pos)
		}
		routeEst, err := routeEstimator.Estimate(shop.Pos, drop)
		if err != nil {
			routeEst = route.FallbackEstimate(shop.Pos, drop)
		}
		km := int64(math.Max(0, routeEst.Meters) / 1000)
		payout, _, _, err := finance.RiderPayoutMinor(1, km)
		if err != nil {
			payout = finance.RiderBasePerDeliveryMajor * finance.MinorPerUnit // floor at the base
		}
		eta := pickupEst.Seconds + route.PickupDwell + routeEst.Seconds + route.DropDwell
		out = append(out, boardRow{
			OrderID:     o.ID,
			ShopName:    o.ShopName,
			ShopLat:     shop.Pos.Lat,
			ShopLng:     shop.Pos.Lng,
			DropLat:     o.Lat,
			DropLng:     o.Lng,
			AddressNote: o.AddressNote,
			Lines:       len(o.Items),
			TotalMinor:  o.TotalMinor,
			PickupKm:    math.Round(pickupEst.Meters) / 1000,
			RouteKm:     math.Round(routeEst.Meters) / 1000,
			ETAMinutes:  int64(math.Round(eta / 60)),
			PayoutMinor: payout,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRiderClaim commits the rider to one job. The engine's guard makes
// it first-wins: a slower rider's claim on the same job is a 409, never a
// shared assignment. The payout locks to what the board showed.
func handleRiderClaim(w http.ResponseWriter, r *http.Request) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	acc, err := dbStore.RiderByID(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "rider account missing — sign in again")
		return
	}
	if !acc.Available {
		writeErr(w, http.StatusConflict, "go on duty before claiming jobs")
		return
	}
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
	if o.Status != orders.StatusConfirmed || o.RiderID != "" {
		writeErr(w, http.StatusConflict, "this order is no longer open for claims")
		return
	}
	shop, err := dbStore.ShopByID(o.ShopID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	routeEst, err := routeEstimator.Estimate(match.Point{Lat: shop.Lat, Lng: shop.Lng}, match.Point{Lat: o.Lat, Lng: o.Lng})
	if err != nil {
		routeEst = route.FallbackEstimate(match.Point{Lat: shop.Lat, Lng: shop.Lng}, match.Point{Lat: o.Lat, Lng: o.Lng})
	}
	km := int64(math.Max(0, routeEst.Meters) / 1000)
	payout, _, _, err := finance.RiderPayoutMinor(1, km)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	claimed, err := orderSvc.Claim(o.ID, acc.ID, int64(math.Round(routeEst.Meters)), payout)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, claimed)
	case errors.Is(err, orders.ErrInvalidTransition):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, orders.ErrUnknownOrder):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

// handleRiderJobs lists the signed-in rider's own claimed jobs, newest first.
func handleRiderJobs(w http.ResponseWriter, r *http.Request) {
	list, err := orderSvc.ForRider(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleRiderTransition is the guarded pick-up/deliver step: only the
// rider who claimed the order may advance it.
func handleRiderTransition(to orders.Status) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if orderSvc == nil {
			writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
			return
		}
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
		if o.RiderID != auth.Subject(r) {
			writeErr(w, http.StatusNotFound, "no such job for this rider")
			return
		}
		moved, err := orderSvc.Transition(o.ID, to)
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, moved)
		case errors.Is(err, orders.ErrInvalidTransition):
			writeErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, orders.ErrUnknownOrder):
			writeErr(w, http.StatusNotFound, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
	}
}
