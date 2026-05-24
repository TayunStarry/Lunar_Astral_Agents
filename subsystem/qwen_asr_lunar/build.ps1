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

# ---------- 图标资源处理 ----------
function Build-IconIfNeeded {
    if (-not (Test-Path "icon.ico")) {
        return
    }

    if (Test-Path "icon.syso") {
        return
    }

    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) { throw "rsrc 图标编译失败" }
}

$PROJECT_DIR = $PSScriptRoot
$OUTPUT_NAME = "Qwen_ASR_Lunar.exe"

# 执行图标处理
Set-Location $PROJECT_DIR
Build-IconIfNeeded

Write-Host "[1/6] Checking Go installation..." -ForegroundColor Yellow
try {
    $goVersion = go version
    Write-Host "  Found: $goVersion" -ForegroundColor Green
} catch {
    Write-Host "  ERROR: Go is not installed or not in PATH" -ForegroundColor Red
    Write-Host "  Please install Go from https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

Write-Host "[3/6] Checking GCC installation (required for CGO)..." -ForegroundColor Yellow
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
    Write-Host "[4/6] Detecting OpenBLAS..." -ForegroundColor Yellow
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
    Write-Host "[4/6] Building without BLAS acceleration" -ForegroundColor Yellow
    Write-Host "  To enable BLAS: .\build.ps1 -UseBLAS" -ForegroundColor Gray
}

Write-Host "[5/6] Building Go binary with CGO..." -ForegroundColor Yellow
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

Write-Host "[6/6] Setting up runtime directory..." -ForegroundColor Yellow
$RUNTIME_DIR = "d:\Lunar_Astral_Agents"
if (-not (Test-Path $RUNTIME_DIR)) {
    New-Item -ItemType Directory -Path $RUNTIME_DIR -Force | Out-Null
}
Copy-Item "$PROJECT_DIR\$OUTPUT_NAME" "$RUNTIME_DIR\$OUTPUT_NAME" -Force
if (Test-Path "$PROJECT_DIR\openblas\lib\libopenblas.dll") {
    Copy-Item "$PROJECT_DIR\openblas\lib\libopenblas.dll" "$RUNTIME_DIR\libopenblas.dll" -Force
    Write-Host "  Copied OpenBLAS DLL to runtime" -ForegroundColor Green
}
Write-Host ""
Write-Host "Runtime directory: $RUNTIME_DIR" -ForegroundColor Cyan
Write-Host "Executable: $OUTPUT_NAME" -ForegroundColor Cyan
Write-Host ""
Write-Host "To run the server:" -ForegroundColor Yellow
Write-Host "  cd $RUNTIME_DIR" -ForegroundColor Gray
Write-Host "  .\$OUTPUT_NAME" -ForegroundColor Gray
Write-Host ""
Write-Host "Environment variables (optional):" -ForegroundColor Yellow
Write-Host "  MODEL_DIR  - Model directory (default: C:\Users\196530\Downloads\Qwen3-ASR-0.6B-0)" -ForegroundColor Gray
Write-Host "  PORT       - Server port (default: 35768)" -ForegroundColor Gray
Write-Host "  QWEN_VERBOSE=2 - Enable timing diagnostics" -ForegroundColor Gray
Write-Host "  QWEN_BF16_CACHE_MB=1024 - BF16 cache size (default: 1024)" -ForegroundColor Gray
Write-Host ""
