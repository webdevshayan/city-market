#!/usr/bin/env bash
# Two-process smoke test proving the decoupling: the standalone web app
# (web module) serves pages while pulling its catalog over HTTP from the
# API-only server (server module) — separate processes standing in for
# separate domains.
set -euo pipefail
export PATH=/tmp/gotc/go/bin:$PATH
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY all_proxy ALL_PROXY
export NO_PROXY="*"
cd /home/z/my-project/server && go build -o /tmp/cms-api .
cd /home/z/my-project/web && go build -o /tmp/cms-web ./cmd/devserver

DATABASE_URL= PORT=8080 /tmp/cms-api & API_PID=$!
API_BASE_URL=http://localhost:8080 /tmp/cms-web & WEB_PID=$!
trap 'kill $API_PID $WEB_PID 2>/dev/null || true' EXIT
sleep 1

echo "--- API root (JSON index) ---"
curl -s http://localhost:8080/ | head -c 120; echo
echo "--- API products (first bytes) ---"
curl -s http://localhost:8080/api/v1/products | head -c 120; echo
echo "--- WEB shop page: product + badge + CSP ---"
BODY=$(curl -s http://localhost:3000/shop)
echo "$BODY" | grep -o 'Halal Beef Boneless' | head -1
echo "$BODY" | grep -oE 'Cart \([0-9]+\)' | head -1
curl -sI http://localhost:3000/shop | grep -i content-security-policy | head -c 80; echo
echo "--- WEB healthz + sitemap ---"
curl -s http://localhost:3000/healthz; echo
curl -s http://localhost:3000/sitemap.xml | grep -c '<loc>'
echo "--- WEB 404 is the branded page (API's is JSON) ---"
curl -s http://localhost:3000/nope | grep -o 'Page not found' | head -1
echo "SMOKE OK"
