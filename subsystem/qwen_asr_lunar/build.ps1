# Qwen ASR Server Build Script for Windows
# Usage: .\build.ps1 [-UseBLAS] [-Release] [-Profile]
param(
    [switch]$UseBLAS = $true,
    [switch]$Release = $true,
    [switch]$Profile = $false
)

$ErrorActionPreference = "Stop"

Write-Host "=== Qwen ASR Server Build Script ===" -ForegroundColor Cyan
Write-Host ""

$PROJECT_DIR = $PSScriptRoot
$OUTPUT_NAME = "asr_lunar.exe"

Write-Host "[1/5] Checking Go installation..." -ForegroundColor Yellow
try {
    $goVersion = go version
    Write-Host "  Found: $goVersion" -ForegroundColor Green
} catch {
    Write-Host "  ERROR: Go is not installed or not in PATH" -ForegroundColor Red
    Write-Host "  Please install Go from https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

Write-Host "[2/5] Checking GCC installation (required for CGO)..." -ForegroundColor Yellow
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
$env:CGO_LDFLAGS_ALLOW = "(-Wl,-flto|-Wl,--gc-sections|-fopenmp|-Wl,.*)"

$cflags = "-Wall -O3 -march=native -mtune=native -ffast-math -ftree-vectorize -funroll-loops"
$ldflags = "-lm -lpthread"

if ($Release) {
    $cflags += " -ffunction-sections -fdata-sections"
    $ldflags += " -Wl,--gc-sections"
}

if ($Profile) {
    $cflags += " -fprofile-generate"
    $ldflags += " -fprofile-generate"
    Write-Host "  Profile-guided optimization: GENERATE mode" -ForegroundColor Magenta
}

if ($UseBLAS) {
    Write-Host "[3/5] Detecting OpenBLAS..." -ForegroundColor Yellow
    $blasFound = $false
    $blasLibPaths = @(
        "$PROJECT_DIR\openblas\lib\libopenblas.a",
        "D:\mingw64\lib\libopenblas.a",
        "C:\msys64\mingw64\lib\libopenblas.a",
        "C:\Tools\OpenBLAS\lib\libopenblas.a"
    )
    foreach ($path in $blasLibPaths) {
        if (Test-Path $path) {
            Write-Host "  Found OpenBLAS: $path" -ForegroundColor Green
            $blasLibDir = Split-Path $path -Parent
            $blasBaseDir = Split-Path $blasLibDir -Parent
            $cflags += " -DUSE_BLAS -fopenmp"
            $cflags += " -I$PROJECT_DIR\openblas\include"
            $ldflags += " -L$blasLibDir"
            $ldflags += " -lopenblas -fopenmp"
            $blasFound = $true
            break
        }
    }
    if (-not $blasFound) {
        Write-Host "  WARNING: OpenBLAS not found, building without BLAS acceleration" -ForegroundColor Yellow
        Write-Host "  Install: pacman -S mingw-w64-ucrt-x86_64-openblas (MSYS2)" -ForegroundColor Gray
    }
} else {
    Write-Host "[3/5] Building without BLAS acceleration" -ForegroundColor Yellow
    Write-Host "  To enable BLAS: .\build.ps1 -UseBLAS" -ForegroundColor Gray
}

Write-Host "[4/5] Building Go binary with CGO..." -ForegroundColor Yellow
Set-Location $PROJECT_DIR

$env:CGO_CFLAGS = $cflags
$env:CGO_LDFLAGS = $ldflags

Write-Host "  CFLAGS: $cflags" -ForegroundColor DarkGray
Write-Host "  LDFLAGS: $ldflags" -ForegroundColor DarkGray

go build -o $OUTPUT_NAME -ldflags="-s -w" .

if ($LASTEXITCODE -eq 0) {
    Write-Host "  Build successful: $OUTPUT_NAME" -ForegroundColor Green
} else {
    Write-Host "  ERROR: Build failed" -ForegroundColor Red
    exit 1
}

Write-Host "[5/5] Setting up runtime directory..." -ForegroundColor Yellow
if (-not (Test-Path "$PROJECT_DIR\output")) {
    New-Item -ItemType Directory -Path "$PROJECT_DIR\output" | Out-Null
}
Copy-Item "$PROJECT_DIR\$OUTPUT_NAME" "$PROJECT_DIR\output\$OUTPUT_NAME" -Force
if (Test-Path "$PROJECT_DIR\openblas\lib\libopenblas.dll") {
    Copy-Item "$PROJECT_DIR\openblas\lib\libopenblas.dll" "$PROJECT_DIR\output\libopenblas.dll" -Force
    Write-Host "  Copied OpenBLAS DLL to output" -ForegroundColor Green
}
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
Write-Host "  QWEN_VERBOSE=2 - Enable timing diagnostics" -ForegroundColor Gray
Write-Host "  QWEN_BF16_CACHE_MB=1024 - BF16 cache size (default: 1024)" -ForegroundColor Gray
Write-Host ""
