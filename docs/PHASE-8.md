# Phase 8 — Citizen accounts: history that follows the phone

Status: **complete, awaiting review**. Every app now has the account flow
it needs: shop owners and riders register and sign in (phase 7), the admin
keeps token-only login by design, and the citizen app — the one gap —
gains register/sign-in and order history without ever forcing an account
on a customer.

## The rule this phase follows

Ordering is a guest activity. A citizen browses, checks out and pays cash
on delivery with no account at all, exactly as before. An account exists
for one reason: attaching order history to a phone number so it survives
a device change. That is why the placement endpoint treats the bearer
token as optional in a deliberate way — no header is a valid guest order,
a valid citizen token attaches the account, and a token that fails to
resolve is a 401 rather than a silent downgrade of a signed-in request to
a guest one (losing history quietly would be worse than an error).

## What changed

- **Server**: `citizens` table (unique phone index backs the duplicate
  check against races), `citizen` as the third auth role through the same
  bcrypt/session machinery as shops and riders, `orders.citizen_id` as an
  additive migration, and `GET /v1/citizen/orders` — the account's history,
  newest first. Guest orders carry an empty `citizen_id` and belong to
  nobody.
- **Shared client**: `registerCitizen`, `citizenOrders(token)`, and an
  optional bearer on `placeOrder` — the server answers all three roles
  with the same `account` shape, so one parser serves every sign-in screen.
- **Citizen app**: a session gate (token kept in `shared_preferences`)
  wraps the catalog. Guests see a Sign in button on the app bar; a
  signed-in citizen sees the account menu with My orders (tap through to
  the live status screen) and Sign out. Registration asks for a name, a
  Pakistani mobile number and a password — nothing else — and the
  sign-in screen says plainly that an account is optional.
- **Admin**: unchanged, deliberately — token login only (`X-Admin-Token`),
  no citizen-style accounts for the console.

## Verification

- Server 9/9 packages green: citizen register/login, duplicate-phone 409
  (also across dialing forms), role isolation in both directions, attached
  placement recording `citizen_id`, guest orders excluded from history,
  broken token on placement → 401.
- Flutter analyze clean everywhere; 33 widget/unit tests green across the
  five packages — citizen's new suite covers guest checkout with no token
  (bearer recorded empty), registration landing back signed in, login +
  My orders history, signed-in checkout attaching `token-fixture`, sign
  out back to guest mode, plus the existing stock/filter/error screens.
