package site

// Sitemap and robots: generated from the live catalog so every filter page
// that actually has products is discoverable, and empty combinations never
// get crawled.
import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
)

// urlset/urlEntry are the sitemap protocol structures.
type urlEntry struct {
	Loc string `xml:"loc"`
}

type urlset struct {
	XMLName xml.Name   `xml:"urlset"`
	Ns      string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

// sitemapPaths lists every indexable path: static pages, the shop, and every
// filter combination that actually has products.
func (s *Site) sitemapPaths(ctx context.Context) []string {
	view := s.catalog(ctx)
	paths := []string{"/", "/privacy", "/terms", "/shop"}
	for _, c := range view.categories() {
		catPath := "/shop/" + c
		paths = append(paths, catPath)
		for _, b := range view.brands() {
			if len(view.filter(c, b, "")) == 0 {
				continue
			}
			brandPath := catPath + "/" + b
			paths = append(paths, brandPath)
			for _, sz := range view.sizes() {
				if len(view.filter(c, b, sz)) > 0 {
					paths = append(paths, brandPath+"/"+sz)
				}
			}
		}
	}
	return paths
}

func (s *Site) handleSitemap(w http.ResponseWriter, r *http.Request) {
	urls := make([]urlEntry, 0, 16)
	for _, p := range s.sitemapPaths(r.Context()) {
		urls = append(urls, urlEntry{Loc: s.base + p})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if _, err := io.WriteString(w, xml.Header); err == nil {
		if err := enc.Encode(urlset{
			Ns:   "http://www.sitemaps.org/schemas/sitemap/0.9",
			URLs: urls,
		}); err != nil {
			log.Printf("render sitemap: %v", err)
		}
	}
}

// handleRobots serves /robots.txt, pointing crawlers at the sitemap.
func (s *Site) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /static/\n\nSitemap: %s/sitemap.xml\n", s.base)
}
