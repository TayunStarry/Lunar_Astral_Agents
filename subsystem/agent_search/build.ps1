# =============================================================================
# build_test.ps1 - agent_search test binary build script
# Locates a Go toolchain and builds cmd/search_test/search_test.exe.
#
# Usage:
#   .\build_test.ps1               # build search_test.exe (default)
#   .\build_test.ps1 -CheckAll     # also build module packages ./...
#   .\build_test.ps1 -Clean        # clean Go build cache first
#   .\build_test.ps1 -Verbose      # verbose build output
# =============================================================================

param(
    [switch]$CheckAll,       # also compile the module packages ./...
    [switch]$Clean,          # clean Go build cache before building
    [switch]$Verbose         # verbose build output
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path

# --- 1. Locate Go toolchain ------------------------------------------------
function Find-Go {
    # 1) Go on PATH
    $cmd = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    # 2) Scan user SDK dir and common install paths, pick the highest version
    $found = @()
    if ($env:USERPROFILE) {
        Get-ChildItem (Join-Path $env:USERPROFILE "sdk") -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -like "go*" } |
            ForEach-Object { $p = Join-Path $_.FullName "bin\go.exe"; if (Test-Path $p) { $found += $p } }
    }
    foreach ($c in @(
        "C:\Program Files\Go\bin\go.exe",
        "C:\Go\bin\go.exe",
        "$env:USERPROFILE\go\bin\go.exe"
    )) {
        if ($c -and (Test-Path $c) -and ($found -notcontains $c)) { $found += $c }
    }
    if ($found.Count -eq 0) { return $null }

    # Highest version first
    return ($found | Sort-Object -Descending | Select-Object -First 1)
}

$Go = Find-Go
if (-not $Go) {
    Write-Error "Go toolchain not found. Install Go or add it to PATH."
    exit 1
}
Write-Host "Using Go: $Go"

# --- 2. Clean build cache (optional) ---------------------------------------
if ($Clean) {
    Write-Host "Cleaning Go build cache..."
    & $Go clean -cache -testcache
    if ($LASTEXITCODE -ne 0) { Write-Error "Failed to clean cache"; exit 1 }
}

# --- 3. Build module packages (optional, skipped by default) ---------------
if ($CheckAll) {
    Write-Host "Building module packages ./..."
    $vArgs = @("build")
    if ($Verbose) { $vArgs += "-v" }
    $vArgs += "./..."
    & $Go @vArgs
    if ($LASTEXITCODE -ne 0) { Write-Error "Module build failed"; exit 1 }
}

# --- 4. Build the test binary ----------------------------------------------
$OutDir = Join-Path $Root "cmd\search_test"
$OutExe = Join-Path $OutDir "search_test.exe"
Write-Host "Building test binary -> $OutExe"

$bArgs = @("build")
if ($Verbose) { $bArgs += "-v" }
$bArgs += @("-o", $OutExe, "./cmd/search_test")
& $Go @bArgs
if ($LASTEXITCODE -ne 0) { Write-Error "Test binary build failed"; exit 1 }

Write-Host ""
Write-Host "Build complete: $OutExe"
Write-Host "Run tests: cd cmd\search_test; .\search_test.exe"