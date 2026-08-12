# build_ggml.ps1 - GGML Library Build Script
# Compiles ggml library for Windows, producing static link artifacts
param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [string]$Generator = "",

    [switch]$Clean,

    [string]$TargetOS = "windows",

    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,

    [string]$OutputDir = "",

    [switch]$EnableLog,

    [switch]$EnableVulkan,

    [string]$DllOutputDir = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

$GGML_SRC_DIR = Join-Path $ScriptDir "cpp\ggml"
if (-not $OutputDir) {
    $GGML_BUILD_DIR = Join-Path $GGML_SRC_DIR "build"
} else {
    $GGML_BUILD_DIR = $OutputDir
}

if ($EnableLog) {
    $BuildLogDir = Join-Path $ScriptDir "build_logs"
    if (-not (Test-Path $BuildLogDir)) {
        New-Item -ItemType Directory -Path $BuildLogDir -Force | Out-Null
    }

    $Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $LogFile = Join-Path $BuildLogDir "ggml_build_${Timestamp}.log"
}

function Write-BuildLog {
    param([string]$Message, [string]$Color = "White")
    $timeStamp = Get-Date -Format "HH:mm:ss"
    $logMessage = "[$timeStamp] $Message"
    Write-Host $logMessage -ForegroundColor $Color
    if ($EnableLog) {
        Add-Content -Path $LogFile -Value $logMessage
    }
}

function Test-CommandExists {
    param([string]$Command)
    $null = Get-Command $Command -ErrorAction SilentlyContinue
    return $?
}

function Test-CMakeMinVersion {
    param([string]$RequiredVersion = "3.14")
    try {
        $cmakeVersion = & cmake --version 2>&1 | Select-Object -First 1
        if ($cmakeVersion -match "(\d+\.\d+\.\d+)") {
            $version = [Version]$matches[1]
            $required = [Version]$RequiredVersion
            return $version -ge $required
        }
    } catch {
        return $false
    }
    return $false
}

function Get-BestGenerator {
    if ($Generator) {
        Write-BuildLog "Using specified generator: $Generator" "Cyan"
        return $Generator
    }

    $mingw32 = Get-Command "gcc" -ErrorAction SilentlyContinue

    if ($mingw32) {
        $gccVersion = & gcc --version 2>&1 | Select-Object -First 1
        Write-BuildLog "GCC (MinGW) detected: $gccVersion" "Green"
        return "MinGW Makefiles"
    }

    if (Test-CommandExists "ninja") {
        Write-BuildLog "Ninja build tool detected, using Ninja generator" "Green"
        return "Ninja"
    }

    $msbuild = Get-Command "MSBuild.exe" -ErrorAction SilentlyContinue
    if ($msbuild) {
        Write-BuildLog "MSBuild detected, using Visual Studio generator" "Green"
        return "Visual Studio 17 2022"
    }

    throw "No C/C++ compiler toolchain found. Please install MinGW-w64 or Visual Studio."
}

function Get-CompilerInfo {
    $info = @{}
    $gcc = Get-Command "gcc" -ErrorAction SilentlyContinue
    $gpp = Get-Command "g++" -ErrorAction SilentlyContinue

    if ($gcc -and $gpp) {
        $info.GCC = $gcc.Source
        $info.GPP = $gpp.Source
        $info.Type = "MinGW"
        try {
            $version = & gcc --version 2>&1 | Select-Object -First 1
            $info.Version = $version
        } catch {
            $info.Version = "unknown"
        }
    } else {
        $info.Type = "MSVC"
        $info.Version = "Visual Studio"
    }
    return $info
}

function Test-VulkanSDK {
    $vulkanSDK = $env:VULKAN_SDK
    if ($vulkanSDK -and (Test-Path $vulkanSDK)) {
        Write-BuildLog "Vulkan SDK found: $vulkanSDK" "Green"
        return $true
    }

    $vulkanSDK64 = ${env:VK_SDK_PATH}
    if ($vulkanSDK64 -and (Test-Path $vulkanSDK64)) {
        $env:VULKAN_SDK = $vulkanSDK64
        Write-BuildLog "Vulkan SDK found: $vulkanSDK64" "Green"
        return $true
    }

    $commonPaths = @(
        "C:\VulkanSDK",
        "${env:ProgramFiles}\VulkanSDK",
        "${env:LOCALAPPDATA}\VulkanSDK"
    )
    foreach ($p in $commonPaths) {
        if (Test-Path $p) {
            $env:VULKAN_SDK = $p
            Write-BuildLog "Vulkan SDK found: $p" "Green"
            return $true
        }
    }

    if (Test-CommandExists "glslc") {
        Write-BuildLog "Vulkan SDK found (via glslc in PATH)" "Green"
        return $true
    }

    Write-BuildLog "Vulkan SDK not found - Vulkan acceleration will be disabled" "Yellow"
    return $false
}

function Build-GGML {
    Write-BuildLog "========================================" "Cyan"
    Write-BuildLog "GGML Library Build Start" "Cyan"
    Write-BuildLog "========================================" "Cyan"
    Write-BuildLog "Source Dir:  $GGML_SRC_DIR" "White"
    Write-BuildLog "Build Dir:   $GGML_BUILD_DIR" "White"
    Write-BuildLog "Build Type:  $BuildType" "White"
    Write-BuildLog "Parallel:    $ParallelJobs" "White"
    Write-BuildLog "Log File:    $LogFile" "White"
    Write-BuildLog "========================================" "Cyan"

    if (-not (Test-CommandExists "cmake")) {
        throw "CMake not found. Please install CMake 3.14+ and add it to PATH."
    }

    if (-not (Test-CMakeMinVersion "3.14")) {
        throw "CMake 3.14 or higher is required."
    }

    $cmakeVersion = & cmake --version 2>&1 | Select-Object -First 1
    Write-BuildLog "CMake version: $cmakeVersion" "Green"

    $compilerInfo = Get-CompilerInfo
    Write-BuildLog "Compiler type: $($compilerInfo.Type)" "Green"
    Write-BuildLog "Compiler version: $($compilerInfo.Version)" "Green"
    if ($compilerInfo.GCC) {
        Write-BuildLog "GCC path: $($compilerInfo.GCC)" "Green"
        Write-BuildLog "G++ path: $($compilerInfo.GPP)" "Green"
    }

    $gen = Get-BestGenerator

    if ($Clean -and (Test-Path $GGML_BUILD_DIR)) {
        Write-BuildLog "Cleaning old build directory..." "Yellow"
        Remove-Item -Recurse -Force $GGML_BUILD_DIR
    }

    if (-not (Test-Path $GGML_BUILD_DIR)) {
        New-Item -ItemType Directory -Path $GGML_BUILD_DIR -Force | Out-Null
        Write-BuildLog "Created build directory: $GGML_BUILD_DIR" "Green"
    } else {
        Write-BuildLog "Build directory exists, performing incremental build" "Green"
    }

    $vulkanEnabled = $false
    if ($EnableVulkan) {
        $vulkanEnabled = Test-VulkanSDK
    } else {
        Write-BuildLog "Vulkan acceleration disabled (-EnableVulkan:`$false)" "Yellow"
    }

    $buildTypeUpper = $BuildType.ToUpper()
    $cmakeConfigureArgs = @(
        "-S", $GGML_SRC_DIR,
        "-B", $GGML_BUILD_DIR,
        "-G", $gen,
        "-DCMAKE_BUILD_TYPE=$buildTypeUpper",
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

    if ($vulkanEnabled) {
        $cmakeConfigureArgs += "-DGGML_VULKAN=ON"
        Write-BuildLog "GGML Vulkan GPU acceleration: ENABLED" "Green"
    } else {
        $cmakeConfigureArgs += "-DGGML_VULKAN=OFF"
        Write-BuildLog "GGML Vulkan GPU acceleration: DISABLED" "Yellow"
    }

    if ($compilerInfo.Type -eq "MinGW") {
        $cmakeConfigureArgs += "-DCMAKE_C_COMPILER=$($compilerInfo.GCC)"
        $cmakeConfigureArgs += "-DCMAKE_CXX_COMPILER=$($compilerInfo.GPP)"
    }

    Write-BuildLog "Running CMake configure..." "Yellow"

    $configureOutput = & cmake $cmakeConfigureArgs 2>&1
    $configureExitCode = $LASTEXITCODE

    if ($configureOutput -and $EnableLog) {
        foreach ($line in $configureOutput) {
            Add-Content -Path $LogFile -Value $line
        }
    }

    if ($configureExitCode -ne 0) {
        Write-BuildLog "CMake configure FAILED (exit code: $configureExitCode)" "Red"
        Write-BuildLog "See log file for details: $LogFile" "Red"
        throw "CMake configure phase failed"
    }

    Write-BuildLog "CMake configure completed" "Green"
    Write-BuildLog "Running CMake build... (parallel: $ParallelJobs)" "Yellow"

    $buildArgs = @(
        "--build", $GGML_BUILD_DIR,
        "--config", $buildTypeUpper,
        "--parallel", $ParallelJobs
    )

    $buildOutput = & cmake $buildArgs 2>&1
    $buildExitCode = $LASTEXITCODE

    if ($buildOutput -and $EnableLog) {
        foreach ($line in $buildOutput) {
            Add-Content -Path $LogFile -Value $line
        }
    }

    if ($buildExitCode -ne 0) {
        $errorLines = $buildOutput | Where-Object { $_ -match "error|Error|ERROR|fatal" }
        Write-BuildLog "Build FAILED (exit code: $buildExitCode)" "Red"
        if ($errorLines) {
            Write-BuildLog "Error summary:" "Red"
            foreach ($err in $errorLines) {
                Write-BuildLog "  $err" "Red"
            }
        }
        Write-BuildLog "Full log: $LogFile" "Red"
        throw "GGML build failed"
    }

    Write-BuildLog "GGML build completed" "Green"

    $expectedLibs = @(
        (Join-Path $GGML_BUILD_DIR "src\ggml.a"),
        (Join-Path $GGML_BUILD_DIR "src\ggml-base.a"),
        (Join-Path $GGML_BUILD_DIR "src\ggml-cpu.a")
    )

    $allLibsExist = $true
    foreach ($lib in $expectedLibs) {
        if (Test-Path $lib) {
            $size = (Get-Item $lib).Length
            $sizeKB = [math]::Round($size / 1KB, 1)
            Write-BuildLog "  [OK] $lib ($sizeKB KB)" "Green"
        } else {
            Write-BuildLog "  [MISSING] $lib" "Red"
            $allLibsExist = $false
        }
    }

    if (-not $allLibsExist) {
        Write-BuildLog "WARNING: Some build artifacts not found" "Yellow"
    }

    Write-BuildLog "========================================" "Cyan"
    Write-BuildLog "GGML Library Build Complete!" "Green"
    Write-BuildLog "========================================" "Cyan"

    return @{
        Success = $true
        BuildDir = $GGML_BUILD_DIR
        LibDir = Join-Path $GGML_BUILD_DIR "src"
        BuildType = $BuildType
        LogFile = $LogFile
    }
}

try {
    $result = Build-GGML
    Write-BuildLog "Build succeeded!" "Green"
    Write-BuildLog "GGML version: 0.11.1" "Green"
    Write-BuildLog "Build type: $BuildType" "Green"
    Write-BuildLog "Log file: $LogFile" "Green"
    exit 0
} catch {
    Write-BuildLog "[ERROR] GGML build failed: $_" "Red"
    Write-BuildLog "Full log: $LogFile" "Red"
    exit 1
}