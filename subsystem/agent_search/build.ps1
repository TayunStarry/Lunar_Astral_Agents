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

# --- 1. Locate Go toolchain (shared implementation) -------------------------
$repoRoot = $Root
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "Repository root not found (subsystem\build_common.ps1)" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")

$Go = Resolve-Go -Purpose "building agent_search"
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