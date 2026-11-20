# Phase 2 — The Web Frontend (HTMX + Security + SEO/AEO)

Status: **complete, awaiting review**. This phase ships the public web
experience served by Go: semantic HTML layout, legal pages, the HTMX shop
with pretty shareable URLs, JSON-LD structured data, dynamic sitemap/robots,
and the CSRF middleware — all under the strict CSP started in Phase 1.

## What's in

| Area | Detail |
| --- | --- |
| Layout | One `base.html` layout: canonical + `og:*` meta, semantic `header/nav/main/footer`, global footer linking the legal pages, cart badge |
| Legal pages | `/privacy` (collection, use, sharing, retention, user rights, security) and `/terms` (marketplace role, user conduct, liability, Halal-first refund policy) |
| FAQ / AEO | Visible FAQ with direct answers; `FAQPage` JSON-LD mirroring it; `Organization` JSON-LD on every page |
| Shop | `/shop`, `/shop/{category}`, `/shop/{category}/{brand}`, `/shop/{category}/{brand}/{size}` — pre-filtered, shareable; unknown vocabulary 404s; valid-but-empty shows an empty state |
| Product UI | Product-first grid: image, name, price, Add to Cart — no store/location clutter |
| HTMX | Self-hosted `htmx.min.js` (no CDN); filter chips swap `#shop-main` via `HX-Request` partials; `hx-push-url="true"` keeps pretty URLs in the address bar |
| CSRF | Double-submit cookie: hardened `cm_csrf` (HttpOnly, SameSite=Strict, Secure on https) echoed via `X-CSRF-Token` (htmx `hx-headers` on `<body>`) or `_csrf` form field; constant-time compare; first visit already renders a usable token via the request context |
| Cart | `POST /cart/add` validates the product, updates a bounded ephemeral session cart (Turso replaces it in the DB phase), swaps the badge fragment over htmx or 303-redirects without JS |
| SEO | `/sitemap.xml` generated from the catalog (pages + product-backed filter combos only) and `/robots.txt` (`Disallow: /static/`, sitemap pointer); `ItemList`/`Product` JSON-LD with price + availability on shop pages |

## Configuration

- `PORT` — listen port (Vercel sets it).
- `BASE_URL` — absolute URL base for canonical/OG tags and sitemap URLs
  (default `http://localhost:3000`; set the production domain on Vercel).
  Cookies go `Secure` automatically when the base URL is https.

## Assumptions (flagged)

1. **Currency:** the spec never names one; `catalog.CurrencyCode = "PKR"` is
   the launch-market assumption, single place to change.
2. **Seed catalog:** `server/catalog` ships 8 labeled fixtures so the shop
   UI is real before the database exists; the Turso-backed provider
   implements the same surface in Phase 3.
3. **Carts are ephemeral** in-memory demos (bounded at 50k sessions) — on
   Vercel instances are short-lived; persistence arrives with Turso.

## Verified

- `go vet` clean; full test suite green across `server`, `finance`,
  `catalog`, `csrf`, `cart` — including: tier boundaries and the overflow
  ceilings, pretty-URL filtering + 404s, HTMX partials, CSRF rejection
  (missing/wrong token, unknown product), cart flow end-to-end, sitemap
  content and robots directives, full-layout regression.
- Live smoke: CSRF cookie issued on first GET; token embedded in
  `hx-headers`; POST without token → 403; add-to-cart swaps the badge;
  sitemap lists 19 URLs and never an empty filter page.

## Next: Phase 3 — Matching Engine

Nearest shop/rider matching, Turso (libSQL) wiring behind the catalog and
cart providers, rate limiting on public endpoints, and the `Nitroping`
integration.
