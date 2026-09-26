#!/usr/bin/env bash
# Builds the dashboard into web/dist with nothing but tsc.
set -euo pipefail
cd "$(dirname "$0")"
rm -rf dist
mkdir -p dist/assets
tsc -p tsconfig.json
cp -r static/. dist/
# cache-bust the entry module
v=$(cat dist/assets/*.js dist/assets/*/*.js dist/app.css 2>/dev/null | sha256sum | cut -c1-10)
sed -i "s/__V__/$v/g" dist/index.html
echo "built web/dist ($v)"
