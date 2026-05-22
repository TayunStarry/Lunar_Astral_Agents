# Qwen ASR Server Build Script for Windows
# Usage: .\build.ps1

$ErrorActionPreference = "Stop"

Write-Host "=== Qwen ASR Server Build Script ===" -ForegroundColor Cyan
Write-Host ""

$PROJECT_DIR = $PSScriptRoot
$OUTPUT_NAME = "asr_lunar.exe"

Write-Host "[1/4] Checking Go installation..." -ForegroundColor Yellow
try {
    $goVersion = go version
    Write-Host "  Found: $goVersion" -ForegroundColor Green
} catch {
    Write-Host "  ERROR: Go is not installed or not in PATH" -ForegroundColor Red
    Write-Host "  Please install Go from https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

Write-Host "[2/4] Checking GCC installation (required for CGO)..." -ForegroundColor Yellow
try {
    $gccVersion = gcc --version | Select-Object -First 1
    Write-Host "  Found: $gccVersion" -ForegroundColor Green
} catch {
    Write-Host "  ERROR: GCC is not installed or not in PATH" -ForegroundColor Red
    Write-Host "  Please install MinGW-w64 or TDM-GCC" -ForegroundColor Red
    exit 1
}

$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

Write-Host "[3/4] Building Go binary with CGO..." -ForegroundColor Yellow
Set-Location $PROJECT_DIR
go build -o $OUTPUT_NAME -ldflags="-s -w" .

if ($LASTEXITCODE -eq 0) {
    Write-Host "  Build successful: $OUTPUT_NAME" -ForegroundColor Green
} else {
    Write-Host "  ERROR: Build failed" -ForegroundColor Red
    exit 1
}

Write-Host "[4/4] Setting up runtime directory..." -ForegroundColor Yellow
if (-not (Test-Path "$PROJECT_DIR\output")) {
    New-Item -ItemType Directory -Path "$PROJECT_DIR\output" | Out-Null
}
Copy-Item "$PROJECT_DIR\$OUTPUT_NAME" "$PROJECT_DIR\output\$OUTPUT_NAME" -Force
if (Test-Path "$PROJECT_DIR\static") {
    Copy-Item "$PROJECT_DIR\static" "$PROJECT_DIR\output\static" -Recurse -Force
}
Write-Host ""
Write-Host "Output directory: $PROJECT_DIR\output" -ForegroundColor Cyan
Write-Host "Executable: asr_lunar.exe" -ForegroundColor Cyan
Write-Host ""
Write-Host "To run the server:" -ForegroundColor Yellow
Write-Host "  cd output" -ForegroundColor Gray
Write-Host "  .\asr_lunar.exe" -ForegroundColor Gray
Write-Host ""
Write-Host "Environment variables (optional):" -ForegroundColor Yellow
Write-Host "  MODEL_DIR  - Model directory (default: C:\Users\196530\Downloads\Qwen3-ASR-0.6B-0)" -ForegroundColor Gray
Write-Host "  PORT       - Server port (default: 35768)" -ForegroundColor Gray
Write-Host ""
