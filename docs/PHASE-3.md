# Phase 3 — Matching Engine & Turso Wiring

Status: **complete, awaiting review**. This phase adds dispatch logic, moves
data into Turso (libSQL) behind small interfaces, and rate-limits the public
API. The frontend, financial engine and security baseline from Phases 1–2
are untouched apart from the wiring they consume.

## Matching engine (`server/match`)

- **Haversine** great-circle distance (antipodal-safe, mean radius 6,371 km),
  validated against externally known distances — London–Paris ≈ 343.5 km and
  1° of latitude/longitude ≈ 111.19 km — not against itself.
- **Nearest-active-place selection** with deterministic ID tie-breaking
  (dispatch is reproducible), plus `SortByDistance` ranked lists. Inactive
  candidates (closed shop, off-duty rider) are always skipped.
- **Endpoints** (rate-limited, JSON):
  - `GET /api/v1/match/shop?lat=&lng=[&product=]` — nearest active shop;
    with `product`, restricted to the shop stocking it.
  - `GET /api/v1/match/rider?lat=&lng=` — nearest available rider.
  - `400` on malformed/out-of-range coordinates, `404` when nothing is
    available, `503` if the dispatch source fails; whole-metre distances.

## Turso wiring (`server/store`)

- `DATABASE_URL` (`libsql://…`) connects through the **pure-Go libsql
  driver** — no CGO, the same binary builds for Vercel.
- **Idempotent schema**: `shops`, `riders`, `products` (with `shop_id`),
  `cart_items`.
- **`SeedIfEmpty`** loads the labeled launch fixtures on first boot: 3 shops
  (synthetic demo coordinates), 4 riders (one deliberately off duty so the
  skip-inactive path is exercised), 8 products mapped to shops. Already
  populated tables are never rewritten.
- **Everything sits behind interfaces** so wiring is a one-line choice:
  `catalog.Provider` (`NewSeed` / `NewDB`), `cart.Store`
  (`NewMemory` / `NewDB`), `placesSource` (fixture adapter / `*store.Store`).
- `main()` → `wireDatabase()`: with `DATABASE_URL` set it opens, migrates,
  seeds-if-empty and switches all three backends to Turso; **any failure
  degrades to seed fixtures with a loud log line** — the site never dies on
  boot over a transient database problem (verified live: a broken DSN boots
  the full site on fixtures).
- **Carts persist across restarts** with the DB backend — proven live: add
  to cart, kill the server, reboot, same session still shows `Cart (1)`.
- Local development without Turso: `go build -tags sqlite` embeds a pure-Go
  sqlite driver so `DATABASE_URL=file:/tmp/dev.db` works; the default
  production build speaks remote `libsql://` only and stays lean.

## Rate limiting (`server/ratelimit`)

- Per-IP token buckets: public API (finance + match) **10 r/s, burst 20**;
  cart posts **2 r/s, burst 8**; `429` responses carry `Retry-After`.
- Keyed on the first `X-Forwarded-For` hop (Vercel fronts TLS) with a
  `RemoteAddr` fallback; the bucket map is capped (10k) with an hourly
  sweep of idle entries; the clock is injectable so refill behaviour is
  tested exactly instead of raced.
- **Honest limitation:** on Vercel this is per-instance (instances are
  ephemeral). It blunts abuse; a durable cross-instance limit needs shared
  state and is deferred until traffic demands it.

## Testing strategy

- **Unit** (CI): full suite green across all 9 packages. DB code is tested
  on `file::memory:` DSNs — the libsql driver routes local files to any
  registered sqlite driver, so the *tests* import `modernc.org/sqlite`;
  the production binary never links it.
- **Integration** (build tag `integration`, not in CI — no database secret
  there): `TestLiveTursoRoundTrip` verifies schema, seed-if-empty
  idempotency and all readers against a real persistent database.
- **Live verification performed for this phase:**
  1. Integration test green on a real on-disk database.
  2. Server booted with `DATABASE_URL` →
     `catalog, carts and dispatch data served from Turso`; shop grid,
     sitemap and match endpoints all answered from DB data.
  3. Cart restart persistence: add → kill → reboot → same session badge
     still `Cart (1)`.
  4. Failure path: an unreachable database logs the reason and serves the
     seed fixtures at full fidelity.

> Note: the local `.env` carries a dev `file:` DATABASE_URL. A remote
> `libsql://` round-trip needs real Turso credentials, which are not (and
> should not be) committed — set `DATABASE_URL` to the Turso URL on
> Vercel and the same code path runs unchanged. Every DB interaction is
> driver-swappable by design.

## Assumptions / decisions

1. Fixture coordinates are synthetic demo data (labeled), not real shops.
2. The DB catalog is a boot-time snapshot; live refresh arrives with the
   admin console (Phase 4).
3. The Phase 2 notes mentioned a "Nitroping integration" — that was an
   error in my notes (no such component exists in the spec); corrected here.
4. Cart writes are single-statement atomic per item; multi-item checkout
   consistency (orders, stock) arrives with the orders phase.

## Next: Phase 4 — Flutter apps

Citizen / shop / rider / admin apps on the same API, plus the admin console
managing shops, riders, catalog and commission schedules.
