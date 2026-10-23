#!/bin/bash
# 日常 Mac 候选包：前端 → production core → 现有打包脚本。
# 只负责构建打包；退出旧版、切入口脚本、保留回滚仍按原方式手工操作。
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

sha=$(git rev-parse HEAD)
if [[ -n "${CANDIDATE_SHA:-}" && "${CANDIDATE_SHA}" != "$sha" ]]; then
  echo "CANDIDATE_SHA 与 HEAD 不一致：${CANDIDATE_SHA} ≠ ${sha}" >&2
  exit 1
fi
export CANDIDATE_SHA="$sha"
short=${sha:0:8}

echo "==> 1/3 构建前端"
(cd frontend && npm run build)

echo "==> 2/3 构建 build/bin/cst-core（production，macOS 12）"
export SDKROOT="${SDKROOT:-/Library/Developer/CommandLineTools/SDKs/MacOSX15.5.sdk}"
if [[ ! -d "$SDKROOT" ]]; then
  echo "找不到 SDK：${SDKROOT}" >&2
  exit 1
fi
export MACOSX_DEPLOYMENT_TARGET=12.0
export CGO_CFLAGS="-mmacosx-version-min=12.0"
export CGO_CXXFLAGS="-mmacosx-version-min=12.0"
export CGO_LDFLAGS="-mmacosx-version-min=12.0"
go build -tags production -buildvcs=true -trimpath \
  -ldflags "-X main.commit=${sha} -X main.version=mac-${short}" \
  -o build/bin/cst-core .

echo "==> 3/3 打包"
python3 scripts/package-mac-candidate.py

app="${repo_root}/build/bin/CST Mac ${short}.app"
built=$(go version -m "${app}/Contents/MacOS/cst-core" | awk '$1=="build" && $2 ~ /^vcs.revision=/ {sub("vcs.revision=", "", $2); print $2}')
modified=$(go version -m "${app}/Contents/MacOS/cst-core" | awk '$1=="build" && $2 ~ /^vcs.modified=/ {sub("vcs.modified=", "", $2); print $2}')
if [[ "$built" != "$sha" ]]; then
  echo "包内核心提交号不符：${built:-未知} ≠ ${sha}" >&2
  exit 1
fi
if [[ "$modified" != "false" ]]; then
  echo "警告：工作树有未提交改动，包内核心不完全等于提交 ${short}" >&2
fi

echo
echo "包：${app}"
echo "core 内嵌提交：${built}（vcs.modified=${modified}）"
echo "manifest：${app}/Contents/Resources/build-manifest.json"
