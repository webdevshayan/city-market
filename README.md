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
apps/citizen   Flutter customer app (Phase 4)
apps/shop      Flutter shop dashboard (Phase 4)
apps/rider     Flutter rider app (Phase 4)
apps/admin     Flutter admin console (Phase 4)
server         Go HTTP server — API now, HTMX web frontend in Phase 2
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

Security groundwork (full suite in Phase 2): every response carries a strict
`Content-Security-Policy` with no inline-script allowance plus
`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and
`Permissions-Policy` headers; HTML is rendered exclusively through Go's
context-aware `html/template`.

## Phases

1. **Financial engine & scaffolding** — this phase.
2. **Web frontend** — HTMX + pretty URLs + legal pages + SEO/AEO (JSON-LD,
   sitemap, robots.txt) + full CSRF middleware.
3. **Matching engine** — nearest shop/rider logic.
4. **Flutter apps** — citizen, shop, rider, admin.

Each phase lands as a pull request describing what changed, how it was
tested, and what to verify before merging.
