// The Vercel Go runtime entry point for the public web app. The runtime
// compiles each api/*.go file as a serverless function — it requires the
// package handler convention with an exported Handler(w, r) — and calls
// Handler for every request routed here by vercel.json's rewrites. All
// logic lives in the site package; this file is the thinnest possible
// shell — which is also why it deliberately has no func main: the runtime
// generates its own wrapper (go vet and go test compile this package
// fine; only a plain go build would want a main).
package handler

import (
	"net/http"
	"os"

	"github.com/webdevshayan/city-market/web/site"
)

// handler is assembled once per instance and reused across requests.
var handler http.Handler = site.New(site.Options{
	BaseURL:    os.Getenv("WEB_BASE_URL"),
	APIBaseURL: os.Getenv("API_BASE_URL"),
	CartSecret: os.Getenv("CART_SECRET"),
})

// Handler is the Vercel Go runtime entry point.
func Handler(w http.ResponseWriter, r *http.Request) {
	handler.ServeHTTP(w, r)
}
