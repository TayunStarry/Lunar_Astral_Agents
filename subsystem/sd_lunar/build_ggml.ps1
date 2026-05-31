# build_ggml.ps1 - sd_lunar GGML Library Build Script
# Pre-builds ggml with Vulkan, avoiding ExternalProject deadlock in main build

param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",
    [switch]$Clean,
    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

$GGML_SRC_DIR = Join-Path $ScriptDir "cpp\ggml"
$GGML_BUILD_DIR = Join-Path $ScriptDir "cpp\build_ggml"

function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host "  [$([DateTime]::Now.ToString('HH:mm:ss'))] $Message" -ForegroundColor $Color
}

Write-Host "========================================" -ForegroundColor Magenta
Write-Host "  SD Lunar - GGML Library Pre-Build" -ForegroundColor Magenta
Write-Host "========================================" -ForegroundColor Magenta
Write-Step "Source: $GGML_SRC_DIR" "White"
Write-Step "Build:  $GGML_BUILD_DIR" "White"
Write-Step "Type:   $BuildType" "White"

if (-not (Test-CommandExists "cmake")) {
    throw "CMake not found"
}

$gcc = Get-Command "gcc" -ErrorAction SilentlyContinue
$gpp = Get-Command "g++" -ErrorAction SilentlyContinue
if (-not $gcc -or -not $gpp) {
    throw "MinGW-w64 GCC not found"
}
Write-Step "GCC: $($gcc.Source)" "Green"
Write-Step "G++: $($gpp.Source)" "Green"

$vulkanSDK = $env:VULKAN_SDK
if (-not $vulkanSDK) {
    $vulkanSDK = "C:\VulkanSDK\1.4.350.0"
}
if (Test-Path $vulkanSDK) {
    $env:VULKAN_SDK = $vulkanSDK
    Write-Step "Vulkan SDK: $vulkanSDK" "Green"
} else {
    Write-Step "Vulkan SDK not found - Vulkan disabled" "Yellow"
}

if ($Clean -and (Test-Path $GGML_BUILD_DIR)) {
    Write-Step "Cleaning build directory..." "Yellow"
    Remove-Item -Recurse -Force $GGML_BUILD_DIR
}

New-Item -ItemType Directory -Path $GGML_BUILD_DIR -Force | Out-Null

$cmakeArgs = @(
    "-S", $GGML_SRC_DIR,
    "-B", $GGML_BUILD_DIR,
    "-G", "MinGW Makefiles",
    "-DCMAKE_BUILD_TYPE=$BuildType",
    "-DCMAKE_C_COMPILER=$($gcc.Source)",
    "-DCMAKE_CXX_COMPILER=$($gpp.Source)",
    "-DBUILD_SHARED_LIBS=OFF",
    "-DGGML_STATIC=ON",
    "-DGGML_BUILD_TESTS=OFF",
    "-DGGML_BUILD_EXAMPLES=OFF",
    "-DGGML_NATIVE=ON",
    "-DGGML_OPENMP=ON",
    "-DGGML_CUDA=OFF",
    "-DGGML_METAL=OFF",
    "-DGGML_BLAS=OFF",
    "-DGGML_BACKEND_DL=OFF",
    "-DGGML_CPU_ALL_VARIANTS=OFF",
    "-DGGML_CPU_KLEIDIAI=OFF",
    "-DGGML_LLAMAFILE=OFF",
    "-DGGML_CPU_HBM=OFF",
    "-DGGML_LTO=OFF",
    "-DGGML_CCACHE=OFF"
)

if ($vulkanSDK -and (Test-Path $vulkanSDK)) {
    $cmakeArgs += "-DGGML_VULKAN=ON"
    Write-Step "Vulkan GPU: ENABLED" "Green"
} else {
    $cmakeArgs += "-DGGML_VULKAN=OFF"
}

Write-Step "Configuring CMake..." "Yellow"
& cmake $cmakeArgs 2>&1 | Select-Object -Last 8
if ($LASTEXITCODE -ne 0) { throw "CMake configure failed" }

Write-Step "Building GGML (parallel: $ParallelJobs)..." "Yellow"
& cmake --build $GGML_BUILD_DIR --config $BuildType --parallel $ParallelJobs 2>&1
if ($LASTEXITCODE -ne 0) { throw "GGML build failed" }

Write-Step "Checking build artifacts..." "Yellow"
$expectedLibs = @(
    (Join-Path $GGML_BUILD_DIR "src\ggml.a"),
    (Join-Path $GGML_BUILD_DIR "src\ggml-base.a"),
    (Join-Path $GGML_BUILD_DIR "src\ggml-cpu.a")
)

if (Test-Path (Join-Path $GGML_BUILD_DIR "src\ggml-vulkan\ggml-vulkan.a")) {
    $expectedLibs += (Join-Path $GGML_BUILD_DIR "src\ggml-vulkan\ggml-vulkan.a")
}

$allGood = $true
foreach ($lib in $expectedLibs) {
    $name = Split-Path $lib -Leaf
    if (Test-Path $lib) {
        Write-Step "  [OK] $name" "Green"
    } else {
        Write-Step "  [MISSING] $name" "Red"
        $allGood = $false
    }
}

Write-Host "========================================" -ForegroundColor Green
Write-Host "  GGML Pre-Build Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
exit 0