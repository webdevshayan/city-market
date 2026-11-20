#!/usr/bin/env bash
# Builds the ONE minified stylesheet the Go server serves at /static/styles.css.
# Scans server templates + Go source. No CDN: the output is committed to the
# repository and served from our own static assets.
set -euo pipefail
cd "$(dirname "$0")/../web"

if [ -f package-lock.json ]; then
  npm ci
else
  npm install
fi

npx tailwindcss -c tailwind.config.js -i input.css -o ../server/static/styles.css --minify
echo "styles.css bytes: $(wc -c < ../server/static/styles.css)"
