// The citizen's signed-in surface: order history. Ordering itself is
// deliberately open to guests — a citizen bearer token on placement only
// attaches history, and an absent token is a perfectly valid guest order.
package main

import (
	"net/http"

	"github.com/webdevshayan/city-market/server/auth"
)

func citizenBearer(next http.HandlerFunc) http.Handler {
	return auth.Middleware(dbStore, auth.RoleCitizen)(next)
}

// citizenSubjectFor resolves the OPTIONAL citizen token on order placement:
// no Authorization header → guest order (empty subject, proceed); a header
// that fails to resolve to a citizen account → 401 before anything is
// written, never a silent downgrade of a signed-in request to a guest one.
func citizenSubjectFor(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := auth.BearerOf(r)
	if raw == "" {
		return "", true
	}
	role, subject, err := dbStore.SessionSubject(auth.HashToken(raw))
	if err != nil || role != auth.RoleCitizen {
		writeErr(w, http.StatusUnauthorized, "your session expired — sign in again, or remove the account to order as a guest")
		return "", false
	}
	return subject, true
}

// handleCitizenOrders serves the signed-in citizen's order history, newest
// first — the My Orders screen.
func handleCitizenOrders(w http.ResponseWriter, r *http.Request) {
	if orderSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "orders require Turso — set DATABASE_URL")
		return
	}
	list, err := orderSvc.ForCitizen(auth.Subject(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}
