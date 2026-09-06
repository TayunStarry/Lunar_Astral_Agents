# build.ps1 - Kokoro TTS 构建脚本
param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "[Check] 检查构建环境..." -ForegroundColor Cyan

if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    throw "未找到 Go 环境，请安装 Go (https://golang.org/dl/)"
}
Write-Host "  [OK] $(go version)" -ForegroundColor Green

if ($TargetOS -eq "windows") {
    if (-not (Get-Command "gcc" -ErrorAction SilentlyContinue)) {
        throw "未找到 GCC 编译器，CGO 支持需要 GCC (请安装 MinGW-w64 或 TDM-GCC)"
    }
    Write-Host "  [OK] $(gcc --version | Select-Object -First 1)" -ForegroundColor Green
}

Write-Host "[Build] 编译 Kokoro_TTS_Lunar.exe ..." -ForegroundColor Yellow

Push-Location $ScriptDir

$env:CGO_ENABLED = "1"
$env:GOOS = $TargetOS
$env:GOARCH = $TargetArch

$goOutput = go build -o "..\..\Kokoro_TTS_Lunar.exe" -ldflags "-s -w" 2>&1
$goExitCode = $LASTEXITCODE

if ($goExitCode -ne 0) {
    Write-Host $goOutput -ForegroundColor Red
    Pop-Location
    throw "Go build failed (exit code: $goExitCode)"
}

$exePath = Join-Path $ScriptDir "..\..\Kokoro_TTS_Lunar.exe"
if (Test-Path $exePath) {
    $exeSize = [math]::Round((Get-Item $exePath).Length / 1MB, 2)
    Write-Host "  [OK] Kokoro_TTS_Lunar.exe ($exeSize MB)" -ForegroundColor Green
}

Pop-Location
Write-Host "[OK] 构建完成" -ForegroundColor Green
exit 0
