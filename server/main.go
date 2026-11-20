// Command server is City Market's HTTP server. It is a plain net/http server
// so it runs unchanged locally (go run ./server) and on Vercel's Go runtime,
// which runs a standard Go server from the project root and expects it to
// listen on the PORT environment variable.
//
// Phase 1 scope: health check, the Halal financial engine API, and one
// html/template-rendered page proving the template + stylesheet pipeline.
// Phase 2 adds the full HTMX web frontend, legal pages, SEO/AEO and the
// complete security suite (CSRF middleware, strict CSP).
package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/webdevshayan/city-market/server/finance"
)

//go:embed templates static
var content embed.FS

var homeTpl = template.Must(template.ParseFS(content, "templates/home.html"))

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	addr := ":" + port
	log.Printf("city-market server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, secureHeaders(newMux())))
}

func newMux() *http.ServeMux {
	staticRoot, err := fs.Sub(content, "static")
	if err != nil {
		log.Fatalf("static assets unavailable: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", noDirectoryListing(http.FileServer(http.FS(staticRoot)))))
	mux.HandleFunc("GET /{$}", handleHome)
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/finance/ujrah", handleUjrah)
	mux.HandleFunc("GET /api/v1/finance/rider", handleRiderPayout)
	mux.HandleFunc("GET /api/v1/finance/checkout-fee", handleCheckoutFee)
	mux.HandleFunc("GET /api/v1/finance/simulate-month", handleSimulateMonth)
	return mux
}

// secureHeaders is the Phase-1 groundwork for the full Phase-2 security
// suite: every response carries a restrictive CSP with no inline script
// allowance, plus the standard no-sniff / frame / referrer guards.
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

// noDirectoryListing blocks index rendering for directory paths so only real
// files under /static/ are ever served (review finding M2).
func noDirectoryListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Title   string
		Tagline string
		Year    int
	}{
		Title:   "City Market",
		Tagline: "100% Halal delivery, fair to shops and riders",
		Year:    time.Now().Year(),
	}
	if err := homeTpl.Execute(w, data); err != nil {
		log.Printf("render home: %v", err)
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "city-market-server"})
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
