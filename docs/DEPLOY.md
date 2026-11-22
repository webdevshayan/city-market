# Production deploys — Vercel from CI (main only)

Both deployments ship to Vercel automatically: every push to `main` runs
the test jobs, and when all four are green the `deploy-server` and
`deploy-web` jobs publish production. Nothing else triggers the pipeline —
no pull-request runs, no branch builds — so production always takes code
from exactly one place: green `main`. `workflow_dispatch` lets you re-run
the pipeline manually from the Actions tab (e.g. redeploy after a Vercel
hiccup); it never deploys anything the tests did not pass.

```
push main ──> CI: server · tailwind · webapp · flutter
                  │ all green
                  ├──> deploy-server ──> Vercel project "server" (API domain)
                  └──> deploy-web    ──> Vercel project "web"    (public domain)
```

## One-time setup

1. Create the two Vercel projects (they mirror the repo's root directories):

   ```bash
   # in the Vercel dashboard or with the CLI, twice:
   #   project "city-market-server"  ← Root Directory: server
   #   project "city-market-web"     ← Root Directory: web
   ```

   With the CLI: run `vercel link` inside `server/` and then inside `web/`,
   answering with the right project name for each. Both `vercel.json` files
   already declare the Go framework preset; no project settings need
   changing.

2. Grab the IDs — after `vercel link`, each directory contains
   `.vercel/project.json` with `orgId` and `projectId` (or read them from
   the dashboard: Project → Settings → General → Project ID; Org ID from
   Team Settings). **Do not commit `.vercel/`** — it is already gitignored;
   the CI reads the IDs from secrets instead.

3. Create the Vercel tokens: Vercel dashboard → Account Settings → Tokens →
   scope them to the team (one per deploy job keeps blast radius small).
   Then set **five credentials** in GitHub (Settings → Secrets and
   variables → Actions). Tokens belong in the **Secrets** tab; the IDs can
   live in either tab — the workflow reads each as `vars.NAME ||
   secrets.NAME`:

   | Name | Tab | Value |
   | --- | --- | --- |
   | `VERCEL_TOKEN_FOR_CM_SERVER` | Secrets | Vercel token used by the server deploy job |
   | `VERCEL_TOKEN_FOR_CM_WEB` | Secrets | Vercel token used by the web deploy job |
   | `VERCEL_ORG_ID` | Variables or Secrets | your Vercel org/team ID |
   | `VERCEL_PROJECT_ID_SERVER` | Variables or Secrets | project ID of the `server` project |
   | `VERCEL_PROJECT_ID_WEB` | Variables or Secrets | project ID of the `web` project |

4. That's it. Until the credentials exist the deploy jobs **skip with a
   notice** naming whatever is missing, and CI stays green; the next push
   to `main` after setting them performs the first production deploys.

## Runtime environment (set once per Vercel project)

These live in Vercel's project settings (or `vercel env add`), not in
GitHub secrets — they configure the running deployment:

- **server** project: `DATABASE_URL` (Turso), `ADMIN_TOKEN`.
- **web** project: `API_BASE_URL` (the server deployment's URL),
  `WEB_BASE_URL` (its own canonical domain — makes cookies `Secure` and
  drives the sitemap), `CART_SECRET` (long random string; carts reset on
  deploys while unset).

## How a deploy works

Each deploy job runs `vercel deploy --prod` from the project's root
directory (`server/` or `web/`), linking by `VERCEL_ORG_ID` +
`VERCEL_PROJECT_ID` from secrets — no committed `.vercel` directory — and
Vercel builds remotely with the Go framework preset both `vercel.json`
files declare. Rollbacks are Vercel-native: promote any previous
deployment from the project's Deployments tab.

## Note on pull requests

PRs get no CI by design (the pipeline's only triggers are `push: main` and
`workflow_dispatch`). If you want pre-merge checks again later, re-add a
`pull_request` trigger — the deploy jobs will still run on main alone.
