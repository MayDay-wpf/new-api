# 本地调试启动脚本：后端 (go run, :3000) + 前端 rsbuild dev server (:5173，/api 代理到 :3000)
# 用法：
#   pwsh scripts/dev.ps1              # 前后端都启动，并打开浏览器
#   pwsh scripts/dev.ps1 -WebOnly     # 只启动前端（后端已在跑）
#   pwsh scripts/dev.ps1 -ApiOnly     # 只启动后端
#   pwsh scripts/dev.ps1 -NoBrowser   # 不自动打开浏览器
param(
  [switch]$WebOnly,
  [switch]$ApiOnly,
  [switch]$NoBrowser,
  [int]$WebPort = 5173
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$web = Join-Path $root 'web'
$shell = if (Get-Command pwsh -ErrorAction SilentlyContinue) { 'pwsh' } else { 'powershell' }

function Start-DevWindow([string]$title, [string]$dir, [string]$cmd) {
  $script = "`$Host.UI.RawUI.WindowTitle = '$title'; Set-Location '$dir'; $cmd"
  Start-Process $shell -ArgumentList '-NoExit', '-Command', $script | Out-Null
}

if (-not $WebOnly) {
  # main.go 会 embed web/dist，首次运行前需要有构建产物
  if (-not (Test-Path (Join-Path $web 'dist/index.html'))) {
    Write-Host '未找到 web/dist，先构建一次前端...' -ForegroundColor Yellow
    Push-Location $web
    try { bun install; bun run build } finally { Pop-Location }
  }
  Write-Host '启动后端: http://localhost:3000' -ForegroundColor Cyan
  Start-DevWindow 'new-api backend' $root 'go run main.go'
}

if (-not $ApiOnly) {
  if (-not (Test-Path (Join-Path $web 'node_modules'))) {
    Write-Host '安装前端依赖...' -ForegroundColor Yellow
    Push-Location $web
    try { bun install } finally { Pop-Location }
  }
  Write-Host "启动前端: http://localhost:$WebPort" -ForegroundColor Cyan
  Start-DevWindow 'new-api web' $web "bun run dev -- --port $WebPort"

  if (-not $NoBrowser) {
    Start-Sleep -Seconds 5
    Start-Process "http://localhost:$WebPort"
  }
}

Write-Host '已在新窗口中启动，关闭对应窗口即可停止服务。' -ForegroundColor Green
