#!/usr/bin/env bash
# 本地调试启动脚本：同时运行 Go 后端和前端开发服务器，Ctrl+C 一并退出。
# 用法：./scripts/dev.sh              （默认后端 3000、前端 5173）
#      PORT=3001 WEB_PORT=5174 ./scripts/dev.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

API_PORT="${PORT:-3000}"
WEB_PORT="${WEB_PORT:-5173}"

for cmd in go bun; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[dev] 缺少依赖：$cmd" >&2
    exit 1
  fi
done

if [[ ! -d web/node_modules ]]; then
  echo "[dev] 首次运行，安装前端依赖..."
  (cd web && bun install)
fi

if [[ ! -f web/dist/index.html ]]; then
  echo "[dev] 生成后端 go:embed 所需的前端产物 web/dist ..."
  (cd web && bun run build)
fi

backend_pid=""
frontend_pid=""

cleanup() {
  trap - EXIT INT TERM
  echo
  echo "[dev] 正在停止..."
  if [[ -n "$frontend_pid" ]]; then
    kill -- "-$frontend_pid" 2>/dev/null || true
  fi
  if [[ -n "$backend_pid" ]]; then
    kill -- "-$backend_pid" 2>/dev/null || true
  fi
  wait 2>/dev/null || true
  echo "[dev] 已停止"
}
trap cleanup EXIT INT TERM

# 让两个子进程各自成为进程组组长，退出时可以整组终止（含 go run 编译出的子进程）。
set -m

echo "[dev] 后端：http://localhost:${API_PORT}"
go run . &
backend_pid=$!

echo "[dev] 前端：http://localhost:${WEB_PORT}（/api、/v1、/mj、/pg 代理到 ${API_PORT}）"
(
  cd web
  export VITE_REACT_APP_SERVER_URL="http://localhost:${API_PORT}"
  exec bun run dev -- --port "$WEB_PORT"
) &
frontend_pid=$!

set +m

# 任一进程退出就停掉另一个，避免只剩半套服务。
while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$frontend_pid" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$backend_pid" 2>/dev/null; then
  echo "[dev] 后端进程已退出，停止前端" >&2
else
  echo "[dev] 前端进程已退出，停止后端" >&2
fi
