// The admin's subscription console: mark a shop paid for a month (the
// payment arrives manually — cash, EasyPaisa or JazzCash — so this is a
// bookkeeping record, never a charge), and read every shop's subscription
// state beside its current-month settlement.
package main

import (
	"net/http"
	"regexp"
	"time"

	"github.com/webdevshayan/city-market/server/auth"
	"github.com/webdevshayan/city-market/server/orders"
)

var monthFormat = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

type payShopBody struct {
	PaidThrough string `json:"paid_through"`
	PaymentNote string `json:"payment_note"`
}

// handleAdminPayShop marks one shop paid through a month. The month may be
// any "YYYY-MM" up to two years out — prepaying several months ahead is
// normal for a bazaar shop with seasonal income.
func handleAdminPayShop(w http.ResponseWriter, r *http.Request) {
	if !adminWrite(w) {
		return
	}
	var b payShopBody
	if err := decodeStrict(w, r, &b); err != nil {
		return
	}
	if !monthFormat.MatchString(b.PaidThrough) {
		writeErr(w, http.StatusBadRequest, `paid_through must be "YYYY-MM"`)
		return
	}
	now := time.Now().UTC()
	farFuture := now.AddDate(2, 0, 0)
	if b.PaidThrough < auth.CurrentMonth(now) || b.PaidThrough > farFuture.Format("2006-01") {
		writeErr(w, http.StatusBadRequest, "paid_through must be between the current month and two years out")
		return
	}
	id := r.PathValue("id")
	if err := dbStore.SetShopPaid(id, b.PaidThrough, b.PaymentNote); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	acc, err := dbStore.ShopByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such shop")
		return
	}
	writeJSON(w, http.StatusOK, shopJSON(acc))
}

// handleAdminSubscriptions lists every shop with its subscription state and
// the current-month settlement beside it — the console's one-call table.
func handleAdminSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !adminWrite(w) {
		return
	}
	accounts, err := dbStore.ShopAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	settlements := map[string]orders.ShopSettlement{}
	if orderSvc != nil {
		snap, err := orderSvc.Stats()
		if err == nil {
			for _, sh := range snap.Shops {
				settlements[sh.ID] = sh
			}
		}
		// A stats failure still serves the subscription rows; the settlement
		// columns just come out zeroed rather than killing the view.
	}
	now := time.Now().UTC()
	out := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		set := settlements[a.ID]
		out = append(out, map[string]any{
			"id":                a.ID,
			"name":              a.Name,
			"active":            a.Active,
			"phone":             a.Phone,
			"paid_through":      a.PaidThrough,
			"payment_note":      a.PaymentNote,
			"eligible":          a.Active && auth.PaidThroughCovers(a.PaidThrough, now),
			"month_orders":      set.Orders,
			"month_sales_minor": set.SalesMinor,
			"ujrah_minor":       set.UjrahMinor,
			"net_minor":         set.NetMinor,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
