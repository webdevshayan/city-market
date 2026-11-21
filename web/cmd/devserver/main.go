// Command devserver runs the City Market web app locally:
//
//	PORT=3000 \
//	API_BASE_URL=http://localhost:8080 \
//	WEB_BASE_URL=http://localhost:3000 \
//	go run ./cmd/devserver
//
// On Vercel the same pages are served by api/index.go through the Go
// runtime — this binary exists only for local development.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/webdevshayan/city-market/web/site"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	opts := site.Options{
		BaseURL:    os.Getenv("WEB_BASE_URL"),
		APIBaseURL: os.Getenv("API_BASE_URL"),
		CartSecret: os.Getenv("CART_SECRET"),
	}
	apiNote := opts.APIBaseURL
	if apiNote == "" {
		apiNote = "(unset — empty shop until API_BASE_URL points at the server)"
	}
	log.Printf("city-market web listening on :%s — catalog API: %s", port, apiNote)
	log.Fatal(http.ListenAndServe(":"+port, site.New(opts)))
}
