# Phase 1 — Financial Engine & Monorepo Scaffolding

Status: **complete, awaiting review**. This phase delivers the repository
skeleton, the Halal-compliant financial engine as a tested Go package, the
Tailwind pipeline that produces exactly one stylesheet, and CI with a strict
storage policy. The web frontend (HTMX, legal pages, SEO/AEO) is Phase 2.

## Business rules (fixed, single source of truth: `server/finance`)

| Rule | Value |
| --- | --- |
| Shop commission (Ujrah), 0–50 monthly orders | 0% |
| Shop commission, 51–200 orders | 5% |
| Shop commission, 201–500 orders | 7% |
| Shop commission, 501+ orders | 9%, capped at 7000 per month |
| Rider payout | 60 per delivery + 15 per km |
| Customer fee | one flat delivery fee at checkout (launch default 25, configurable) |

Design decisions:

- Money is `int64` **minor units** end to end; no floats touch settlement math.
- Commission rounding floors, so any fraction of a minor unit stays with the
  merchant — the platform never overcharges a shop.
- The 7000 cap is applied to the 9% tier (the literal reading of the spec:
  "500+: 9% capped at 7000 max"). If the cap is meant to be global, it is a
  one-line change in `CommissionMinor` plus a test.
- User-supplied amounts are bounded (`finance.MaxSalesMinor` = 1e14 minor,
  deliveries/km ≤ 1e9) so the settlement products can never overflow int64;
  inputs above the ceilings are rejected with 400, never clamped.
- The flat checkout fee's amount is not stated in the spec; 25 is the launch
  default, exposed via `finance.DefaultDeliveryFeeMajor` and the
  `/api/v1/finance/checkout-fee` endpoint so it can be changed centrally.

## HTTP endpoints (verified locally, see PR description for output)

```
GET /healthz
GET /api/v1/finance/ujrah?orders=N&sales_minor=M
GET /api/v1/finance/rider?deliveries=N&km=K
GET /api/v1/finance/checkout-fee
GET /api/v1/finance/simulate-month?orders=N&sales_minor=M&deliveries=D&km=K
GET /                      (html/template-rendered foundation page)
GET /static/styles.css     (single minified stylesheet)
```

Example:

```
curl 'http://localhost:3000/api/v1/finance/ujrah?orders=1000&sales_minor=10000000'
# → commission 7000.00 (9% tier capped), commission_capped: true
```

## Tailwind pipeline

`scripts/build-css.sh` scans `server/templates/**/*.html` and
`server/**/*.go` and emits **one** minified file, `server/static/styles.css`,
which is committed and served by the Go server from embedded assets. No CDN.
The CI `web` job rebuilds it and fails if the output is missing or trivially
small; the artifact is uploaded with `retention-days: 1`.

## Storage policy

- All workflow artifacts: `retention-days: 1`.
- A `cleanup` job force-deletes artifacts older than 24 hours.
- No node_modules, build outputs, or binaries are committed (see
  `.gitignore`); the only generated file intentionally committed is the
  minified stylesheet.

## How to run locally

```
bash scripts/build-css.sh          # rebuild the stylesheet (needs Node 18+)
cd server && go test ./... -v      # Halal math proofs
cd server && go run .              # serves on :3000 (PORT env to change)
```

## Review outcome (autonomous review on PR #1)

Scored 8.4/10 before fixes, 9.6/10 after. Applied on this branch:

- **C1 (critical):** unbounded `sales_minor`/`deliveries`/`km` could overflow
  int64 and wrap settlement amounts negative → input ceilings + 400 +
  exploit-shaped tests (`349a772`).
- **M2:** `/static/` served directory listings → directories now 404, with a
  regression test (`b9977f1`).
- **M1:** stylesheet drift was invisible to CI → the web job now fails when
  the committed CSS differs from a fresh regeneration.
- Accepted notes: rate limiting lands with Phase 3; `Tiers` becomes an
  immutable accessor when the admin console manages schedules.

## Next: Phase 2 — The Web Frontend

Legal pages (`/privacy`, `/terms`) linked from the global footer; HTMX with
`hx-push-url="true"` and pretty URLs (`/shop/:category/:brand/:size`);
JSON-LD (Organization, Product, FAQPage); dynamic `/sitemap.xml` and
`/robots.txt` from the database; full CSRF middleware for HTMX requests and
the strict CSP already started by `secureHeaders` in `server/main.go`.
