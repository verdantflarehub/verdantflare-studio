#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

usage() {
  cat <<'HELP'
用法：bash scripts/build.sh [web|desktop|image]
  web      构建网站程序（默认），输出 build/studio-web
  desktop  构建当前平台桌面程序，输出 build/studio-desktop
  image    构建 Gin 网站镜像，名称由 STUDIO_IMAGE 配置
HELP
}
fail() { echo "构建失败：$*" >&2; exit 1; }
require() { command -v "$1" >/dev/null 2>&1 || fail "缺少 $1，请先安装。"; }

target="${1:-web}"
case "$target" in
  -h|--help) usage; exit 0 ;;
  web|desktop|image|frontend) ;;
  *) usage >&2; fail "未知目标：$target" ;;
esac
# Only used between the frontend and Go stages of Dockerfile.
backend_only=false
if [[ $# -gt 1 ]]; then
  [[ $# -eq 2 && "$target" == web && "$2" == --backend-only ]] || fail '参数不正确。'
  backend_only=true
fi

if [[ "$target" == image ]]; then
  require docker
  exec docker build -f Dockerfile -t "${STUDIO_IMAGE:-verdantflare-studio:gin-dev}" .
fi

if [[ "$target" != frontend ]]; then require go; fi
if [[ "$backend_only" == false ]]; then
  require node
  expected="$(node -p "require('./frontend/package.json').packageManager.split('@')[1]")"
  if command -v corepack >/dev/null 2>&1; then
    package_runner=(corepack pnpm)
  else
    require pnpm
    package_runner=(pnpm)
  fi
  (
    cd frontend
    actual="$("${package_runner[@]}" --version)"
    [[ "$actual" == "$expected" ]] || fail "需要 pnpm $expected，当前为 $actual。"
    "${package_runner[@]}" install --frozen-lockfile
    "${package_runner[@]}" build
  )
fi
[[ "$target" != frontend ]] || exit 0
[[ -s frontend/dist/index.html ]] || fail '缺少前端构建产物。'
mkdir -p build
suffix=''
[[ "$(go env GOOS)" != windows ]] || suffix='.exe'
output="build/studio-${target}${suffix}"
if [[ "$target" == web ]]; then
  CGO_ENABLED=0 go build -trimpath -o "$output" ./cmd/web
else
  go build -trimpath -o "$output" ./cmd/desktop
fi
printf '构建完成：%s/%s\n' "$PROJECT_ROOT" "$output"
