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
server         Go HTTP server — API + HTMX frontend, matching engine, Turso
web            Tailwind build: scans Go templates, emits ONE minified stylesheet
docs           Phase notes and decisions
scripts        build tooling (stylesheet build)
```

## Server

Plain `net/http` — the same binary runs locally and on Vercel's Go runtime
(`server/vercel.json` sets the Go framework preset; the server listens on
`PORT`). Templates and static assets are embedded, so the stylesheet and
HTML ship inside the binary.

```
bash scripts/build-css.sh       # rebuild server/static/styles.css (Node 18+)
cd server && go test ./... -v   # financial engine proofs
cd server && go run .           # serve on :3000
```

Security: every response carries a strict `Content-Security-Policy` with no
inline-script allowance plus `X-Content-Type-Options`, `X-Frame-Options`,
`Referrer-Policy` and `Permissions-Policy` headers; HTML is rendered
exclusively through Go's context-aware `html/template`; state-changing
requests (like the htmx cart) are protected by double-submit-cookie CSRF
middleware with a constant-time token compare; `/static/` never lists
directories. Set `BASE_URL` so canonical tags, Open Graph and the generated
`/sitemap.xml` carry the production domain.

## Phases

1. **Financial engine & scaffolding** — merged (PR #1).
2. **Web frontend** — merged (PR #2): HTMX + pretty shareable URLs, legal
   pages, FAQ + JSON-LD, dynamic sitemap/robots, CSRF middleware.
3. **Matching engine & Turso** — merged (PR #3): nearest shop/rider dispatch,
   libSQL-backed catalog/carts/dispatch data behind small interfaces,
   per-IP rate limiting on the public API.
4. **Flutter apps** — this phase: citizen, shop, rider, admin on a shared
   API client, backed by the new orders/admin JSON API.

Each phase lands as a pull request describing what changed, how it was
tested, and what to verify before merging.
