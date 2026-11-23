# Phase 7 — The Marketplace: accounts, subscriptions, stock, and a rider dispatch board

Status: **complete, awaiting review**. City Market stops being a demo with
hard-coded shop IDs and becomes a marketplace anyone in Johi can join:
shop owners register and manage their own shelves, riders claim delivery
jobs from a map-first board with real estimates before any commitment,
customers order from a specific shop or let the market pick one, and the
admin runs the money — charts on top, and the monthly subscription that
switches a shop's ordering on.

## Why this shape

The phase-4 flow assigned shops and riders automatically at placement,
with no stock and no owners. That is fine for a demo and wrong for a real
bazaar: shops must control their own listings and accept orders themselves,
riders must choose jobs they can actually see (distance, time, payout)
before committing, and the platform's income — the monthly Ujrah — needs a
bookkeeping surface. Payment collection stays manual for now (cash,
EasyPaisa or JazzCash settled with the admin directly); the system's job is
to record it and gate dispatch on it, not to charge anyone.

## What the server gained

- **Accounts** (`server/auth`): bcrypt credentials, bearer sessions stored
  as SHA-256 hashes with a 30-day TTL, role-checked middleware (a shop
  token never opens rider routes), and Pakistani mobile normalization —
  `0300…`, `+92 300…` and `92-300…` all collapse to `92XXXXXXXXXX`.
  `POST /v1/auth/{shop/register,rider/register,login,logout}`.
- **Subscriptions**: `shops.paid_through` ("YYYY-MM") plus a free-text
  payment note. `PATCH /v1/admin/shops/{id}/paid` records the manual
  payment (up to two years ahead — seasonal income is normal);
  `GET /v1/admin/subscriptions` lists every shop's state beside its
  current-month settlement. The eligibility rule is one line: a shop
  receives orders when it is open AND paid through the running month.
- **Stock**: `products.stock`, decremented atomically when a shop accepts
  an order (a short shelf fails the accept and names the product),
  restored when a confirmed order is cancelled, and checked at placement
  so the citizen learns about a short shelf immediately. The public
  catalog overlays live stock on every read.
- **The order flow reworked**: placement takes an optional `shop_id`
  (the citizen picking a shop — every item must belong to it, a mismatch
  is a 409, never a silent reassignment) and an `address_note` (the
  delivery address the rider reads). No rider is assigned at placement
  anymore — the payout on a placed order is provisional and `rider_id`
  stays empty until a rider claims the job.
- **The dispatch board** (`server/route` + rider endpoints): confirmed
  orders with no rider are open jobs. `GET /v1/rider/board` returns each
  rider's personalized estimates — pickup km, route km, ETA minutes, the
  locked payout, and the exact pickup/drop points. Road distance and time
  come from a public OSRM instance with a 3 s ceiling, cached per rounded
  pair, falling back to a calibrated model (haversine × 1.32 road factor
  at 24 km/h town speed plus handover dwell) so the board never stalls.
  `POST /v1/rider/orders/{id}/claim` is first-wins — the loser gets a 409,
  never a shared job — and locks the ledger payout to what the rider
  accepted. pick-up/deliver now require the claiming rider's own token.
- **Charts data**: `GET /v1/admin/stats` carries a 14-day daily series
  (orders, GMV, payouts — UTC buckets, empty days included) so the console
  draws history, not just totals.

## What the apps gained

- **Shop**: register/sign-in (token kept in `shared_preferences`), the
  subscription banner (green "taking orders" or the red wall with exactly
  what to do), the queue with Accept/Cancel and the customer's delivery
  address beside the COD total, and a shelf manager — name, category,
  brand, pack size, PKR price, stock.
- **Rider**: the duty toggle, the board with mini-map previews and the
  full decision set per job, a whole-screen claim map (pickup square, drop
  square, route line, OpenStreetMap tiles — no API key, attribution
  always on), and the claimed-job navigation map.
- **Citizen**: stock-aware catalog with category and shop filters (picking
  a shop chip is the one-tap "order from this shop"), and a checkout
  screen — draggable delivery pin on the map, address note, cart grouped
  per shop with the honest note that a mixed cart becomes one order per
  shop, cash on delivery everywhere. The order screen says "waiting for a
  rider to claim the job" until the board assigns one.
- **Admin**: the stats view plus three hand-painted square charts (orders,
  GMV, payouts per day — each announcing its totals to screen readers) and
  the subscription table with a Mark-paid action (month defaults to the
  current one; the note records how the money arrived).

## The seed moved home

The launch fixtures sat on synthetic Karachi coordinates. The market's
home is **Johi, Dadu District, Sindh** (26°41′31″N 67°36′48″E), so the
seed shops now sit in realistic walking distance of the Johi bazaar —
Madina Halal Store, Al-Karam Grocery & Meat, Bismillah Fresh Mart — with
Sindh rider names and one rider deliberately off duty. New databases seed
into that world; the dispatch orderings are pinned by tests.

## Honest notes

- **Deploying this flips the gate on**: production shops have an empty
  `paid_through`, so the moment this ships they stop receiving orders
  until the admin marks the month paid — one tap per shop in the admin
  console. That is the feature working as specified, but it is worth
  knowing before merging.
- The rider's claimed-job map uses the shop coordinates captured at claim
  time; jobs claimed before this session fall back to the town center pin
  (the drop pin and the address note are always exact).
- OSRM is a public instance; its answers are best-effort and the fallback
  model is a model — estimates, labeled as such, never promises.
- Pre-existing Turso data keeps its old shop names and coordinates; the
  admin console edits them live. Fresh databases get Johi from the start.
- Shop registration pins new shops to Johi's coordinates for now; the
  admin's shop upsert remains the way to fine-tune a pin.

## Verification

- Server: `gofmt`/`go vet` clean, 9 packages green — auth unit tests
  (phone rules, month math, token roles), the shop lifecycle (create →
  accept decrements 5 → 2 → cancel restores 5, double-accept 409), the
  paid gate (auto-mode generic refusal, explicit-shop named refusal),
  rider flow (duty → board estimates → claim → first-wins race → guarded
  steps), route estimator (fallback math, cache hits, TTL, OSRM parsing
  against a stub, failure fallback), admin (pay-shop validation, the
  subscriptions table, series sums equal the ledger totals).
- Flutter: `flutter analyze` clean and widget tests green across all five
  packages — shop (sign-in, banner, accept, product form, sign-out),
  rider (duty toggle, board estimates, claim, pick-up → deliver),
  citizen (stock chips, shop filter, checkout validation, COD order
  shape, error surfacing), admin (stats, charts, mark-paid dialog,
  sign-out), core (client shapes incl. bearer headers and the board JSON).
