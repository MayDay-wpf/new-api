# 发布打包脚本：构建前端 -> 交叉编译 Linux 二进制（嵌入前端）-> 输出到 release/
# 编译参数与 Dockerfile 保持一致（CGO_ENABLED=0、PGO、-s -w、注入版本号）
# 用法：
#   pwsh scripts/build-release.ps1                    # 默认 linux/amd64
#   pwsh scripts/build-release.ps1 -Arch arm64        # 打 arm64 包（aarch64 服务器）
#   pwsh scripts/build-release.ps1 -SkipWeb           # 只改了后端时跳过前端构建（需已有 web/dist）
#   pwsh scripts/build-release.ps1 -Version v1.2.3    # 指定版本号（默认读 VERSION，空则用时间戳）
param(
  [ValidateSet('amd64', 'arm64')]
  [string]$Arch = 'amd64',
  [switch]$SkipWeb,
  [string]$Version
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$web = Join-Path $root 'web'
$releaseDir = Join-Path $root 'release'
$output = Join-Path $releaseDir 'new-api'
$startTime = Get-Date

function Invoke-Step([string]$title, [scriptblock]$action) {
  Write-Host "==> $title" -ForegroundColor Cyan
  & $action
  if ($LASTEXITCODE -ne 0) { throw "$title 失败（exit code $LASTEXITCODE）" }
}

foreach ($cmd in @('go') + $(if ($SkipWeb) { @() } else { @('bun') })) {
  if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "未找到命令 $cmd，请先安装并加入 PATH" }
}

# 版本号：参数 > VERSION 文件 > 时间戳
if (-not $Version) {
  $versionFile = Join-Path $root 'VERSION'
  if (Test-Path $versionFile) { $Version = "$(Get-Content $versionFile -Raw)".Trim() }
  if (-not $Version) { $Version = 'local-' + (Get-Date -Format 'yyyyMMddHHmm') }
}
Write-Host "版本: $Version   目标: linux/$Arch" -ForegroundColor Green

# 1. 前端
if ($SkipWeb) {
  if (-not (Test-Path (Join-Path $web 'dist/index.html'))) { throw '使用了 -SkipWeb，但 web/dist 不存在，请先完整构建一次' }
  Write-Host '==> 跳过前端构建，使用现有 web/dist' -ForegroundColor Yellow
} else {
  Push-Location $web
  try {
    Invoke-Step '安装前端依赖' { bun install --frozen-lockfile }
    $env:DISABLE_ESLINT_PLUGIN = 'true'
    $env:VITE_REACT_APP_VERSION = $Version
    Invoke-Step '构建前端' { bun run build }
  } finally {
    Remove-Item Env:DISABLE_ESLINT_PLUGIN, Env:VITE_REACT_APP_VERSION -ErrorAction SilentlyContinue
    Pop-Location
  }
}

# 2. 后端交叉编译
$envBackup = @{}
$buildEnv = @{ GOWORK = 'off'; CGO_ENABLED = '0'; GOOS = 'linux'; GOARCH = $Arch; GOEXPERIMENT = 'greenteagc' }
Push-Location $root
try {
  $env:GOWORK = 'off'
  Invoke-Step '下载 Go 依赖' { go mod download }
  $pgoDir = (go list -m -f '{{.Dir}}' github.com/MoeForge/moejs | Out-String).Trim()
  $pgo = if ($pgoDir) { Join-Path $pgoDir 'default.pgo' } else { '' }
  $pgoArg = if ($pgo -and (Test-Path $pgo)) { "-pgo=$pgo" } else { Write-Host '未找到 PGO 配置文件，关闭 PGO 继续编译' -ForegroundColor Yellow; '-pgo=off' }

  foreach ($k in $buildEnv.Keys) {
    $envBackup[$k] = [Environment]::GetEnvironmentVariable($k)
    [Environment]::SetEnvironmentVariable($k, $buildEnv[$k])
  }
  New-Item -ItemType Directory -Force $releaseDir | Out-Null
  $ldflags = "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$Version'"
  Invoke-Step '编译后端' { go build $pgoArg -ldflags $ldflags -o $output . }
} finally {
  foreach ($k in $buildEnv.Keys) { [Environment]::SetEnvironmentVariable($k, $envBackup[$k]) }
  Pop-Location
}

# 3. 产物信息
$file = Get-Item $output
$hash = (Get-FileHash $output -Algorithm SHA256).Hash.ToLower()
"$hash  new-api" | Set-Content -Encoding ascii (Join-Path $releaseDir 'new-api.sha256')
$elapsed = [int]((Get-Date) - $startTime).TotalSeconds

Write-Host ''
Write-Host "打包完成（${elapsed}s）" -ForegroundColor Green
Write-Host ("  文件:   {0}" -f $file.FullName)
Write-Host ("  大小:   {0:N1} MB" -f ($file.Length / 1MB))
Write-Host ("  平台:   linux/{0}" -f $Arch)
Write-Host ("  版本:   {0}" -f $Version)
Write-Host ("  SHA256: {0}" -f $hash)
Write-Host ''
Write-Host '上传到服务器后执行 chmod +x new-api，然后在进程守护中重启即可。' -ForegroundColor Yellow
