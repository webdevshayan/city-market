# Phase 4 — Flutter Apps: Citizen, Shop, Rider, Admin

Status: **complete, awaiting review**. This phase ships the four Flutter
apps on a shared Dart API client, and the JSON backend they need — the
phase where the Phase 1 finance engine and the Phase 3 match engine
converge into a real end-to-end order flow.

## Backend: the JSON API the apps consume

The apps cannot scrape HTML, so this phase adds the API surface (all
rate-limited, all honest about failure):

- **Citizen**: `POST /api/v1/orders` (items + coordinates → priced,
  assigned, persisted; 201) and `GET /api/v1/orders/{id}` (96-bit random
  IDs, unguessable). Validation failures are 400; dispatch conflicts
  (multi-shop cart, no open shop stocking the items, no available rider)
  are 409 with the real reason; no database → an honest 503.
- **Shop**: `GET /api/v1/shop/orders?shop_id` (the queue) and
  `POST /api/v1/shop/orders/{id}/confirm`.
- **Rider**: `GET /api/v1/rider/orders?rider_id`, and
  `POST /api/v1/rider/orders/{id}/pick-up` / `.../deliver`.
- **Admin** (`X-Admin-Token`, constant-time compared; unset `ADMIN_TOKEN`
  default-denies the whole console with 503; wrong token 401; the rate
  limiter wraps auth so token guessing is throttled):
  `PUT /api/v1/admin/{shops,riders,products}/{id}` upserts,
  `GET /api/v1/admin/{shops,riders,products}` lists, and
  `GET /api/v1/admin/commission?orders&sales` — the settlement view of the
  fixed Ujrah schedule. Product upserts reload the catalog snapshot (and
  the order service) in-process, so a new product is orderable on the next
  request without a restart.

## The order engine (server/orders)

`Place()` merges duplicate lines, snapshots unit prices, requires every
item to come from one shop (split orders are rejected — a documented
limitation until a later phase), assigns the nearest **open** shop stocking
all items and the nearest **available** rider to that shop, and prices the
order with the finance engine: subtotal + the flat customer delivery fee,
plus the rider payout (60 per delivery + 15 per km of the shop→customer
route). Lifecycle: `placed → confirmed (shop) → picked_up → delivered
(rider)`, cancellable until pickup, with optimistic guarded updates — a
repeated or skipped step is 409, never a silent state change. Shop/rider
names join from the live tables so admin renames appear without rewriting
orders.

## The apps (apps/)

- **`apps/core`** — the dependency-free seam: `ApiClient` (interface, so
  widget tests substitute fakes), `HttpApiClient` (JSON over HTTP), models
  mirroring the server's JSON exactly (snake_case, minor-unit money, the
  same `money()` rendering as the server's `PriceText`), `ApiException`
  carrying the server's real error message, and `testing.dart` — a
  `FakeApi` with recorded calls and status-mutating fixtures.
- **`apps/citizen`** — product-first catalog, quantity steppers, checkout
  dialog with coordinates (defaults tuned for the Android emulator, whose
  `10.0.2.2` reaches the host's localhost), a server-priced order screen
  that names the shop and rider and explains the Halal fee model, live
  status refresh, and errors surfaced as real messages.
- **`apps/shop`** — the shop's order queue with the confirm step.
- **`apps/rider`** — the job list with pick-up/deliver and the
  engine-computed payout per route.
- **`apps/admin`** — token-guarded console: shop upsert (live dispatch),
  product upsert (live catalog), lists, and the commission calculator.

API base URL is injected with `--dart-define=API_BASE_URL=...`. Apps
depend only on `flutter` and `core`; `core` depends only on `http` — no
other packages, so `flutter pub get` resolves deterministically.

## Verification

- **Local (Flutter 3.24.5 stable, real SDK in the sandbox)**:
  `flutter analyze` clean and `flutter test` green for all five packages —
  9 core unit tests + 10 widget tests (citizen 3, shop 2, rider 2, admin 3)
  covering rendering, money formatting, request shapes, error mapping, the
  admin token header, and the full place-order and status-transition
  interactions against `FakeApi`.
- **Go**: full suite green (9 packages), including the orders engine on
  in-memory databases and the API flows end-to-end on `file::memory:` DBs.
- **CI**: new `flutter` job runs pub get + analyze + test for core and all
  four apps on the same Flutter version; the storage-cleanup job waits for
  it.

Two real bugs were caught locally before push, both by the tests this
phase ships: `HttpApiClient._send` accepted a request body but never
attached it (checkout would have POSTed empty bodies), and an arrow
closure returning a `Future` from `setState` tripped Flutter's own
assertion in every app's reload path.

## Honest scope notes

1. **No device run yet**: verification is analyze + widget tests on the
   real SDK — no emulator or APK build in CI (an Android SDK build is the
   natural next step, and adds a heavy runner dependency).
2. The API contract for orders is deliberately minimal (no auth beyond
   unguessable IDs, no pagination); real accounts and tokens arrive with
   multi-tenant hardening.
3. `POST /api/v1/orders` requires the database — orders are durable by
   nature, so without `DATABASE_URL` the endpoint is 503, never a fake.
4. The commission schedule is **fixed by the specification** — the admin
   console views and computes settlements, it does not edit rates.

## Next: Phase 5 — hardening & QA

Real accounts/auth for the apps, order polling or push, APK builds in CI,
and device QA against a deployed Vercel + Turso stack.
