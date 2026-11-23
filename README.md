# City Market

A **100% Halal delivery marketplace**: customers order from certified-Halal
shops, riders deliver, and every fee in the system follows the fixed business
rules below. Nothing here handles riba (interest): the shop commission is a
flat tiered service fee (Ujrah), rider pay is a fixed per-delivery + per-km
wage, and the customer fee is one flat checkout charge.

## Business rules

| Rule | Value |
| --- | --- |
| Shop commission (Ujrah) | 0–50 orders: 0% · 51–200: 5% · 201–500: 7% · 501+: 9% capped at 7000 |
| Rider payout | 60 per delivery + 15 per km |
| Customer fee | one flat delivery fee at checkout |

All amounts are computed in integer minor units by
[`server/finance`](server/finance/finance.go) — the single source of truth,
covered by table-driven tests at every tier boundary.

## Layout

```
apps/core      Shared Dart API client, models and test double for all apps
apps/citizen   Flutter customer app — catalog, flat-fee checkout, order status
apps/shop      Flutter shop dashboard — order queue, confirm step
apps/rider     Flutter rider app — job list, pick-up/deliver, payouts
apps/admin     Flutter admin console — shops, catalog, settlement view
server         Go API server — JSON API (catalog, dispatch, finance, orders, admin) over Turso
web            Standalone public web app — own Go module + own domain, Vercel-deployed
docs           Phase notes and decisions
scripts        build tooling (stylesheet build, split smoke test)
```

## Server & web app

Two deployments, two domains:

**`server/`** — the API. Plain `net/http` (the same binary runs locally and
on Vercel's Go runtime; `server/vercel.json` sets the Go framework preset;
it listens on `PORT`). JSON in, JSON out: catalog, dispatch matching,
finance, orders, admin — `GET /` returns a JSON endpoint index, unknown
paths get JSON 404s. The server serves no HTML.

**`web/`** — the public web app on its own Go module and domain. It fetches
the catalog server-side from the API and renders the storefront (home with
FAQ + JSON-LD, pretty-URL shop, legal pages, sitemap/robots); the visitor
cart is one HMAC-signed cookie, so the app is stateless and
serverless-safe. See `web/README.md` for env vars and deploy steps.

```bash
bash scripts/build-css.sh            # rebuild web/site/static/styles.css (Node 18+)
cd server && go test ./...           # API: finance, match, orders, admin
cd web    && go test ./...           # web app: pages, cart, csrf, catalog source
cd server && PORT=8080 go run .      # API on :8080
cd web    && API_BASE_URL=http://localhost:8080 go run ./cmd/devserver   # site on :3000
bash scripts/smoke-split.sh          # two-process proof of the decoupling
```

Security (both apps): strict CSP with no inline-script allowance plus
`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and
`Permissions-Policy` headers; HTML through context-aware `html/template`;
state-changing requests protected by double-submit-cookie CSRF with a
constant-time compare; `/static/` never lists directories. On the web app,
`WEB_BASE_URL` drives canonical tags and the generated `/sitemap.xml` (and
makes cookies `Secure`); `CART_SECRET` signs cart cookies.

Deploys: the CI pipeline runs on `main` only — the four test jobs gate the
`deploy-server` and `deploy-web` jobs that publish both Vercel projects to
production. One-time setup (two projects, five credentials): `docs/DEPLOY.md`.

## Install the apps

The four Flutter apps ship as signed APKs on the
[Releases page](https://github.com/webdevshayan/city-market/releases) — one
versioned bundle per change to `main`, built automatically
(`docs/RELEASES.md` covers versioning, signing and the one-time release-key
setup).

## Phases

1. **Financial engine & scaffolding** — merged (PR #1).
2. **Web frontend** — merged (PR #2): HTMX + pretty shareable URLs, legal
   pages, FAQ + JSON-LD, dynamic sitemap/robots, CSRF middleware.
3. **Matching engine & Turso** — merged (PR #3): nearest shop/rider dispatch,
   libSQL-backed catalog/carts/dispatch data behind small interfaces,
   per-IP rate limiting on the public API.
4. **Flutter apps** — merged (PR #4): citizen, shop, rider, admin on a
   shared API client, backed by the orders/admin JSON API.
5. **APK releases** — merged: every change to `main` ships all four
   apps as a versioned GitHub Release (`docs/RELEASES.md`).
6. **The split** — merged: the public web moved to a standalone Go
   project in `web/` (own module, own Vercel project and domain), reading
   the catalog from the API; the server became API-only, and the cart/CSRF
   plumbing moved into the web module as a signed-cookie cart.
7. **The marketplace** — merged: shop owners register and manage their
   shelves with stock, riders claim jobs from a map-first dispatch board
   with distance/ETA/payout estimates before committing, customers order
   from a specific shop with a delivery pin and cash on delivery, and the
   admin runs monthly shop subscriptions (payments received manually —
   cash/EasyPaisa/JazzCash — gate dispatch) beside daily charts. The seed
   world moved home to Johi, Dadu District, Sindh (`docs/PHASE-7.md`).
8. **Citizen accounts** — this phase: the customer app gains optional
   register/sign-in and order history that follows the phone, while guest
   checkout stays fully open; the admin console remains token-only
   (`docs/PHASE-8.md`).

Each phase lands as a pull request describing what changed, how it was
tested, and what to verify before merging.
