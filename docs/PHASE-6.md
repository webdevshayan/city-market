# Phase 6 — The Split: public web app decoupled from the API server

Status: **complete, awaiting review**. The public routes move out of the
server into a completely new Golang + Vercel project under `web/` — its own
Go module, its own Vercel project, its own domain. The server becomes a
pure API deployment. Nothing browser-facing and nothing API-facing shares a
process anymore.

## Why

Until now one binary served both the storefront pages and the JSON API.
That couples release cadence (a CSS tweak redeploys the API), scales the
two against each other, and forces one domain to expose both surfaces. The
split gives each side a clean contract: the web app consumes the API over
HTTP exactly like the Flutter apps do, and the server stops pretending to
be a website.

## The new architecture

```
browser ──> WEB (own domain, web/, Vercel Go runtime)
                │   server-side fetches only — the API domain stays
                │   invisible to the browser (no CORS surface)
                ▼
            SERVER (API domain, server/, Vercel Go runtime + Turso)
```

- **`web/`** — module `github.com/webdevshayan/city-market/web`, zero
  imports from the server. `site/` renders home (FAQ + JSON-LD), the
  pretty-URL shop, legal pages, sitemap/robots and static assets from the
  same templates as before (moved with `git mv`). `api/index.go` +
  `vercel.json` are the Vercel entry point (every route rewrites to the
  function). `cmd/devserver` runs it locally.
- **`server/`** — API-only: all `/api/v1/*` routes, a JSON endpoint index
  at `GET /`, JSON 404s, `GET /healthz`. The HTML handlers, embedded
  templates/static, session cookies, and the consumerless `cart` and `csrf`
  packages are gone; `cart_items` leaves the schema for fresh databases.
- **New on the API**: `GET /api/v1/products` (public, rate-limited) — the
  one read endpoint the web app builds its shop from, so there is exactly
  one source of truth for what is for sale.

## What replaced what

| Old (one process) | New (split) |
| --- | --- |
| Session id cookie + `cart.Store` rows | One HMAC-signed cart cookie per visitor — stateless, serverless-safe, tamper-proof, capped (50 × 99); `CART_SECRET`, ephemeral key by default |
| `server/csrf` package | `web/internal/csrf` port (double-submit, constant-time compare) |
| `server/ratelimit` on `/cart/add` | `web/internal/ratelimit` port (per-IP token bucket, 2/s burst 8) |
| In-process catalog provider | `site.CatalogSource` → `GET /api/v1/products`, last-good snapshot cached; stale data on API blips, an honestly empty grid if the API never answered — the web app carries **no catalog of its own**, so no seed drift is possible |
| `BASE_URL` on the server | `WEB_BASE_URL` on the web (canonical, sitemap, Secure cookies) + `API_BASE_URL` pointing at the server deployment |

## Verification

- **web module**: `go vet` + `go test` green — the ported page tests
  (full-layout regression, pretty URLs, HTMX partials, sitemap/robots,
  static listing block), the full CSRF + cart flow end-to-end, cart unit
  tests (tamper, cross-secret, capacity, invalid IDs), catalog-source
  degradation tests (fresh → cached → stale → honest empty), and the
  Vercel `Handler` smoke test.
- **server module**: all seven packages green after the shed; the binary
  builds and `GET /` serves the endpoint index.
- **Two-process smoke** (`scripts/smoke-split.sh`): API on :8080, web on
  :3000 with `API_BASE_URL` set — the web shop page renders the API's
  catalog across the process boundary, carries CSP, its own healthz
  (`city-market-web`), a 19-URL sitemap, and branded 404s while the API
  answers JSON.
- **CI**: new `webapp` job (vet + test, `go-version-file: web/go.mod`);
  the Tailwind job now scans `web/site/**` and emits
  `web/site/static/styles.css` with the same drift check; stylesheet
  rebuilt **before push** so CI passes on the first run.
- **`go vet` note**: `web/api` is `package main` without `func main` — the
  Vercel runtime generates its own wrapper. `go vet`/`go test` compile it
  fine; the CI web job deliberately runs vet + test (not a bare `go build
  ./...`), and the `api` package has its own test to keep it compiled and
  covered.

## Deploy notes

Two Vercel projects: **Root Directory `server`** (existing) and **Root
Directory `web`** (new). Env for the web project: `API_BASE_URL` (the
server deployment's URL), `WEB_BASE_URL` (its own canonical domain),
`CART_SECRET` (long random string). The server keeps `DATABASE_URL` and
`ADMIN_TOKEN`. The Flutter apps keep pointing at the API domain via
`API_BASE_URL` — unaffected by the split.

## Honest scope notes

1. The web app's catalog fetch happens per shop-page render (a few ms on
   the same platform); no CDN caching layer is configured — Vercel's edge
   handles the public traffic and the API snapshot cache absorbs blips.
2. `robots.txt`/`sitemap.xml` now advertise the web domain only — crawler
   semantics stay intact because the URLs in them are exactly the pages
   that exist on that domain.
3. Order placement from the web storefront is a future step (the citizen
   flow lives in the Flutter app today); the split deliberately changes
   nothing about `POST /api/v1/orders`.

## Next

Real accounts/auth, order placement from the web storefront, and cache
headers for the API-backed pages.
