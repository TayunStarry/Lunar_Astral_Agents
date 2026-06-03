# GGUF Metadata Viewer - Build Script
# Environment check, Go dependency resolution and project compilation
#
# Usage:
#   .\build.ps1                       Default build (Windows amd64)
#   .\build.ps1 -TargetOS linux       Cross-compile for Linux
#   .\build.ps1 -TargetArch arm64     Build for ARM64
#   .\build.ps1 -Clean                Clean and rebuild

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    [switch]$Clean
)

$ErrorActionPreference = "Stop"
$ScriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

# ---------- Helpers ----------
function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host "[ ] $Message" -ForegroundColor $Color
}

function Write-OK {
    param([string]$Message)
    Write-Host "[OK] $Message" -ForegroundColor Green
}

function Write-Fail {
    param([string]$Message)
    Write-Host "[FAIL] $Message" -ForegroundColor Red
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[!] $Message" -ForegroundColor Yellow
}

# ---------- Environment Checks ----------
function Check-GoEnvironment {
    Write-Step "Checking Go environment..."

    if (-not (Test-CommandExists "go")) {
        Write-Fail "Go not found. Install from https://golang.org/dl/"
        throw "Go environment not installed"
    }

    $goVersion = go version 2>&1 | Select-Object -First 1
    Write-OK $goVersion
}

function Check-GCCToolchain {
    if ($TargetOS -ne "windows") {
        return
    }

    Write-Step "Checking GCC compiler toolchain..."

    if (-not (Test-CommandExists "gcc")) {
        Write-Warn "GCC compiler not found"
        Write-Warn "CGO support requires GCC (install MinGW-w64 or TDM-GCC)"
        Write-Warn "Download: https://www.msys2.org/"
        throw "GCC compiler not installed"
    }

    $gccVersion = gcc --version 2>&1 | Select-Object -First 1
    Write-OK $gccVersion
}

# ---------- Build Pipeline ----------
try {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Magenta
    Write-Host "  GGUF Metadata Viewer - Build System" -ForegroundColor Magenta
    Write-Host "========================================" -ForegroundColor Magenta
    Write-Host ""
    Write-Host "Target: $TargetOS / $TargetArch" -ForegroundColor Yellow
    Write-Host "Project: $ScriptRoot" -ForegroundColor Yellow
    Write-Host ""

    # Stage 1: Environment
    Write-Host "--- Stage 1: Environment Check ---" -ForegroundColor Yellow
    Check-GoEnvironment
    Check-GCCToolchain
    Write-OK "All environment checks passed"
    Write-Host ""

    # Stage 2: Dependencies
    Write-Host "--- Stage 2: Go Module Dependencies ---" -ForegroundColor Yellow

    Push-Location $ScriptRoot

    if ($Clean) {
        Write-Step "Cleaning build cache..."
        go clean -cache
        Write-OK "Build cache cleaned"
    }

    Write-Step "Running go mod tidy..."
    & go mod tidy 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) {
        Write-Warn "go mod tidy had warnings, continuing..."
    }
    Write-OK "Go module dependencies ready"
    Write-Host ""

    # Stage 3: Compile
    Write-Host "--- Stage 3: Compile Go Binary ---" -ForegroundColor Yellow

    $env:CGO_ENABLED = 1
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    if ($TargetOS -eq "windows") {
        $env:CC = "gcc"
        $env:CXX = "g++"
    }

    $binaryName = if ($TargetOS -eq "windows") { "gguf_metadata_viewer.exe" } else { "gguf_metadata_viewer" }
    $outputPath = Join-Path (Split-Path $ScriptRoot -Parent) $binaryName

    $ldflags = "-s -w"

    Write-Step "CGO_ENABLED=$env:CGO_ENABLED GOOS=$env:GOOS GOARCH=$env:GOARCH"
    Write-Step "Output: $outputPath"

    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )

    Write-Step "Running go build..."
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "Go build failed (exit code: $LASTEXITCODE)"
        Pop-Location
        throw "Go build failed"
    }

    if (Test-Path $outputPath) {
        $fileInfo = Get-Item $outputPath
        $sizeMB = [math]::Round($fileInfo.Length / 1MB, 2)
        Write-OK "Build successful: $outputPath ($sizeMB MB)"
    } else {
        Write-Fail "Output file not found: $outputPath"
        Pop-Location
        throw "Output file missing"
    }

    Pop-Location

    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  GGUF Metadata Viewer - Build Complete!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
}
catch {
    Write-Host ""
    Write-Host "[ERROR] GGUF Metadata Viewer build failed: $_" -ForegroundColor Red
    Write-Host ""
    Pop-Location -ErrorAction SilentlyContinue
    exit 1
}