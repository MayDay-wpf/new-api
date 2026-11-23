#!/usr/bin/env bash
# 发布打包脚本（macOS / Linux）：构建前端 -> 交叉编译 Linux 二进制（嵌入前端）-> 输出到 release/
# 编译参数与 Dockerfile、scripts/build-release.ps1 保持一致（CGO_ENABLED=0、PGO、-s -w、注入版本号）
# 用法：
#   ./scripts/build-release.sh            # 默认 linux/amd64
#   ./scripts/build-release.sh -a arm64   # 打 arm64 包（aarch64 服务器）
#   ./scripts/build-release.sh -s         # 只改了后端时跳过前端构建（需已有 web/dist）
#   ./scripts/build-release.sh -v v1.2.3  # 指定版本号（默认读 VERSION，空则用时间戳）
#   ./scripts/build-release.sh -h         # 查看帮助
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$ROOT_DIR/web"
RELEASE_DIR="$ROOT_DIR/release"
OUTPUT="$RELEASE_DIR/new-api"

ARCH="amd64"
SKIP_WEB=0
VERSION=""

if [[ -t 1 ]]; then
  C_CYAN=$'\033[36m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_RED=$'\033[31m'
  C_OFF=$'\033[0m'
else
  C_CYAN=''
  C_GREEN=''
  C_YELLOW=''
  C_RED=''
  C_OFF=''
fi

step() { printf '%s==> %s%s\n' "$C_CYAN" "$1" "$C_OFF"; }
say() { printf '%s%s%s\n' "$C_GREEN" "$1" "$C_OFF"; }
warn() { printf '%s%s%s\n' "$C_YELLOW" "$1" "$C_OFF" >&2; }
die() {
  printf '%s错误：%s%s\n' "$C_RED" "$1" "$C_OFF" >&2
  exit 1
}

usage() {
  cat <<'EOF'
用法: scripts/build-release.sh [选项]

选项:
  -a, --arch <amd64|arm64>   目标架构（默认 amd64）
  -s, --skip-web             跳过前端构建，复用现有 web/dist
  -v, --version <版本号>     指定版本号（默认读 VERSION 文件，为空则用时间戳）
  -h, --help                 显示本帮助

示例:
  ./scripts/build-release.sh                 # linux/amd64
  ./scripts/build-release.sh -a arm64        # linux/arm64
  ./scripts/build-release.sh -s -v v1.2.3    # 只重编后端并打上指定版本号

产物:
  release/new-api            二进制（前端资源已嵌入）
  release/new-api.sha256     SHA256 校验文件
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
  -a | --arch)
    [[ $# -ge 2 ]] || die "参数 $1 缺少取值（amd64 / arm64）"
    ARCH="$2"
    shift 2
    ;;
  -a=* | --arch=*)
    ARCH="${1#*=}"
    shift
    ;;
  -s | --skip-web)
    SKIP_WEB=1
    shift
    ;;
  -v | --version)
    [[ $# -ge 2 ]] || die "参数 $1 缺少版本号"
    VERSION="$2"
    shift 2
    ;;
  -v=* | --version=*)
    VERSION="${1#*=}"
    shift
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    die "未知参数：${1}（用 -h 查看用法）"
    ;;
  esac
done

case "$ARCH" in
amd64 | arm64) ;;
*) die "不支持的架构：${ARCH}（可选 amd64 / arm64）" ;;
esac

required=(go)
if [[ "$SKIP_WEB" -eq 0 ]]; then
  required+=(bun)
fi
for cmd in "${required[@]}"; do
  command -v "$cmd" >/dev/null 2>&1 || die "未找到命令 ${cmd}，请先安装并加入 PATH"
done

# 版本号：参数 > VERSION 文件 > 时间戳
if [[ -z "$VERSION" && -f "$ROOT_DIR/VERSION" ]]; then
  VERSION="$(tr -d '[:space:]' <"$ROOT_DIR/VERSION")"
fi
[[ -n "$VERSION" ]] || VERSION="local-$(date +%Y%m%d%H%M)"

START_TS="$(date +%s)"
say "版本: $VERSION   目标: linux/$ARCH"

# 1. 前端
if [[ "$SKIP_WEB" -eq 1 ]]; then
  [[ -f "$WEB_DIR/dist/index.html" ]] || die "使用了 -s/--skip-web，但 web/dist 不存在，请先完整构建一次"
  warn '==> 跳过前端构建，使用现有 web/dist'
else
  step '安装前端依赖'
  (cd "$WEB_DIR" && bun install --frozen-lockfile)

  step '构建前端'
  (cd "$WEB_DIR" && DISABLE_ESLINT_PLUGIN=true VITE_REACT_APP_VERSION="$VERSION" bun run build)
fi

# 2. 后端交叉编译
step '下载 Go 依赖'
(cd "$ROOT_DIR" && GOWORK=off go mod download)

step '解析 PGO 配置'
pgo_dir="$(cd "$ROOT_DIR" && GOWORK=off go list -m -f '{{.Dir}}' github.com/MoeForge/moejs)"
PGO_ARG='-pgo=off'
if [[ -n "$pgo_dir" && -f "$pgo_dir/default.pgo" ]]; then
  PGO_ARG="-pgo=$pgo_dir/default.pgo"
else
  warn '未找到 PGO 配置文件，关闭 PGO 继续编译'
fi

mkdir -p "$RELEASE_DIR"
LDFLAGS="-s -w -X 'github.com/QuantumNous/new-api/common.Version=$VERSION'"

step "编译后端（linux/${ARCH}）"
(cd "$ROOT_DIR" && env GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" GOEXPERIMENT=greenteagc \
  go build "$PGO_ARG" -ldflags "$LDFLAGS" -o "$OUTPUT" .)

# 3. 产物信息
if command -v sha256sum >/dev/null 2>&1; then
  HASH="$(sha256sum "$OUTPUT" | awk '{print $1}')"
else
  HASH="$(shasum -a 256 "$OUTPUT" | awk '{print $1}')"
fi
printf '%s  new-api\n' "$HASH" >"$RELEASE_DIR/new-api.sha256"

SIZE_MB="$(awk -v bytes="$(wc -c <"$OUTPUT" | tr -d ' ')" 'BEGIN { printf "%.1f", bytes / 1048576 }')"
ELAPSED=$(($(date +%s) - START_TS))

echo
say "打包完成（${ELAPSED}s）"
printf '  文件:   %s\n' "$OUTPUT"
printf '  大小:   %s MB\n' "$SIZE_MB"
printf '  平台:   linux/%s\n' "$ARCH"
printf '  版本:   %s\n' "$VERSION"
printf '  SHA256: %s\n' "$HASH"
echo
warn '上传到服务器后执行 chmod +x new-api，然后在进程守护中重启即可。'
