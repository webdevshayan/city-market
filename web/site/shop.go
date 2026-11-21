package site

// handleShop serves the product grid at pretty, shareable filter URLs:
// /shop, /shop/{category}, /shop/{category}/{brand}, /shop/{category}/{brand}/{size}.
// Unknown vocabulary 404s so engines never index impossible filter pages;
// valid-but-empty combinations render a friendly empty grid. Product data
// comes from the catalog source (the City Market API), never from a local
// copy.
import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
)

func (s *Site) handleShop(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	brand := r.PathValue("brand")
	size := r.PathValue("size")
	view := s.catalog(r.Context())
	if !view.validFilter(category, brand, size) {
		if r.Header.Get("HX-Request") == "true" {
			// htmx does not swap 4xx responses by default, so the grid stays
			// as-is; direct loads of a hand-crafted bad URL still get the
			// full branded 404 page below.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.render404(w, r)
		return
	}
	data := s.baseData(r, shopTitle(category, brand, size),
		"Certified-Halal products with transparent prices. Product-first listings: image, name, price, add to cart.")
	data.CategoryLabel = Display(category)
	data.BrandLabel = Display(brand)
	data.SizeLabel = Display(size)
	data.Products = view.filter(category, brand, size)
	data.JSONLD = itemListJSONLD(data.BaseURL, data.Products)

	catLinks := []FilterLink{{Name: "All", URL: "/shop", Active: category == ""}}
	for _, c := range view.categories() {
		catLinks = append(catLinks, FilterLink{Name: Display(c), URL: "/shop/" + c, Active: c == category})
	}
	data.CatLinks = catLinks
	if category != "" {
		catBase := "/shop/" + category
		for _, b := range view.brands() {
			data.BrandLinks = append(data.BrandLinks, FilterLink{Name: Display(b), URL: catBase + "/" + b, Active: b == brand})
		}
		if brand != "" {
			brandBase := catBase + "/" + brand
			for _, sz := range view.sizes() {
				data.SizeLinks = append(data.SizeLinks, FilterLink{Name: Display(sz), URL: brandBase + "/" + sz, Active: sz == size})
			}
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		// HTMX swaps just the shop-main region; hx-push-url keeps the pretty
		// URL in the address bar, and direct loads of that URL render the
		// full page with the same pre-filtered state.
		renderHTML(w, http.StatusOK, tplShopMain, "shopMain", data)
		return
	}
	renderHTML(w, http.StatusOK, tplShop, "base", data)
}

func shopTitle(category, brand, size string) string {
	var parts []string
	for _, d := range []string{Display(category), Display(brand), Display(size)} {
		if d != "" {
			parts = append(parts, d)
		}
	}
	if len(parts) == 0 {
		return "Shop — City Market"
	}
	return strings.Join(parts, " · ") + " — City Market"
}

// itemListJSONLD renders the ItemList of Products (price + availability) for
// answer engines on shop pages.
func itemListJSONLD(base string, products []Product) template.JS {
	elems := make([]map[string]any, len(products))
	for i, p := range products {
		elems[i] = map[string]any{
			"@type":    "ListItem",
			"position": i + 1,
			"item": map[string]any{
				"@type": "Product",
				"name":  p.Name,
				"image": base + p.Image,
				"offers": map[string]any{
					"@type":         "Offer",
					"price":         p.PriceText(),
					"priceCurrency": CurrencyCode,
					"availability":  "https://schema.org/InStock",
				},
			},
		}
	}
	b, _ := json.Marshal(map[string]any{
		"@context":        "https://schema.org",
		"@type":           "ItemList",
		"name":            "City Market products",
		"itemListElement": elems,
	})
	return template.JS(b)
}
