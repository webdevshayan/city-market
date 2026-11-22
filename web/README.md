# city-market-web — the public web app

The City Market **public web app**: a standalone Go project on its own
module (`github.com/webdevshayan/city-market/web`), deployed to Vercel on
**its own domain**, fully decoupled from the API server (`../server`, its
own deployment and domain).

```
browser ──> web (this project, public domain)
                │  server-side fetches only (no CORS, no browser API calls)
                ▼
            server (API domain): GET /api/v1/products, POST /api/v1/orders
```

## What lives here

- `site/` — the storefront: home with FAQ + JSON-LD, pretty-URL shop
  (`/shop/{category}/{brand}/{size}`), legal pages, sitemap/robots, static
  assets, and the visitor cart.
- `internal/cart` — the cart is **one HMAC-signed cookie** (stateless,
  serverless-safe, tamper-proof, capped at 50 items × 99 qty).
- `internal/csrf`, `internal/ratelimit` — security primitives ported from
  the server when the app became a separate deployment.
- `api/index.go` + `vercel.json` — the Vercel Go runtime entry point; every
  route rewrites to it.
- `cmd/devserver` — local development binary.

## Architecture rules

1. **No database, no business rules.** The catalog is fetched server-side
   from the API (`GET /api/v1/products`) and cached in memory; when the API
   blips the last good snapshot is served, and a never-answered API renders
   an honestly empty shop — the web app invents nothing.
2. **No client-side API calls.** Pages and the cart POST are same-origin;
   cross-origin browser calls would need CORS on the server and leak the
   API surface — the server-side fetch keeps the API domain invisible to
   the browser.
3. **Money is never recomputed here.** Prices render from API data; order
   pricing, Ujrah, payouts stay in `server/finance`.

## Environment variables (Vercel project settings)

| Variable | Meaning | Default (local) |
| --- | --- | --- |
| `API_BASE_URL` | The City Market server deployment's base URL | unset → empty shop |
| `WEB_BASE_URL` | Canonical/sitemap base of THIS domain | `http://localhost:3000` |
| `CART_SECRET` | HMAC key for the cart cookie (set a long random string in production) | ephemeral per-instance key (carts reset on deploy) |
| `PORT` | Listen port (Vercel sets it; only used by `cmd/devserver`) | `3000` |

Cookies become `Secure` automatically when `WEB_BASE_URL` starts with
`https://`.

## Deploy

Automatic: every green push to `main` deploys this app to Vercel
production from CI (alongside the API project — see `../docs/DEPLOY.md`
for the one-time setup: two Vercel projects, five credentials). Manual
fallback, if you ever need it:

```bash
cd web && vercel link && vercel env add API_BASE_URL && vercel --prod
```

## Develop locally

```bash
# terminal 1 — the API
cd ../server && go run .                        # listens on :3000

# terminal 2 — the web app
cd web && API_BASE_URL=http://localhost:8080 go run ./cmd/devserver
# (start the API with PORT=8080, or point API_BASE_URL wherever it listens)
```

## Test

```bash
go vet ./... && go test ./...
```

## Stylesheet

One minified Tailwind stylesheet is committed at `site/static/styles.css`
and served from this app's own `/static/`. Rebuild after touching templates
or Go sources (they feed the class scan):

```bash
../scripts/build-css.sh
```
