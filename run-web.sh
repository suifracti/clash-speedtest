#!/usr/bin/env bash
# Source + Web launcher for macOS/Linux. Build the frontend once before running.
set -euo pipefail
migration_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$migration_root"
command -v go >/dev/null || { echo 'Install Go 1.25+ (Windows checkpoint used 1.27.0).'; exit 1; }
command -v node >/dev/null || { echo 'Install Node.js, then run npm ci and npm run build in frontend/.'; exit 1; }
node scripts/prepare-legacy-assets.mjs
if [[ ! -f frontend/dist/index.html ]]; then
  echo 'Frontend assets missing. Run: (cd frontend && npm ci && npm run build)'
  exit 1
fi
migration_commit="$(git rev-parse --short=12 HEAD)"
exec go run -buildvcs=false -ldflags="-X main.version=mac-migration-20261005 -X main.commit=$migration_commit" . --web --port 18474 --listen 127.0.0.1 --browser none "$@"
