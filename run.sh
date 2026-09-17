#!/usr/bin/env bash
# Clash-SpeedTest launcher (macOS / Linux)
# Runs from source with `go run`. No binary is built.
#
# Usage:
#   ./run.sh
#   ./run.sh -c "https://example.com/subscribe?token=xxx&flag=meta"
#   ./run.sh -fast
#
# Without -c, the program opens the airport menu:
# select / add / update / delete airports, then pick countries to test.
#
# First time on macOS: chmod +x run.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

find_go() {
  if command -v go >/dev/null 2>&1; then
    command -v go
    return 0
  fi
  local candidate
  for candidate in \
    /usr/local/go/bin/go \
    /opt/homebrew/bin/go \
    /usr/local/bin/go \
    "$HOME/go/bin/go" \
    "$HOME/.local/go/bin/go"
  do
    if [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

GO_BIN="$(find_go || true)"
if [ -z "$GO_BIN" ]; then
  cat <<'EOF'
未找到 Go。本脚本用 go run 从源码启动，需要先安装 Go 1.24+。
  macOS:  brew install go
  其它:   https://go.dev/doc/install
装完后重新打开终端再运行本脚本。
EOF
  exit 1
fi

if [ -z "${GOPROXY:-}" ]; then
  export GOPROXY="https://goproxy.cn,direct"
fi

echo "从源码启动 clash-speedtest（go run，不生成二进制）..."
if [ "$#" -gt 0 ]; then
  exec "$GO_BIN" run . "$@"
fi
exec "$GO_BIN" run .
