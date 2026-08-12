param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [string]$Generator = "",

    [switch]$Clean,

    [switch]$SkipGGML,

    [string]$TargetOS = "windows",

    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,

    [string]$OutputDir = "",

    [switch]$EnableLog,

    [switch]$EnableVulkan,

    [string]$DllOutputDir = ""
)

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

$CPP_SRC_DIR = Join-Path $ScriptDir "cpp"
$GGML_SRC_DIR = Join-Path $CPP_SRC_DIR "ggml"
$GGML_BUILD_DIR = Join-Path $GGML_SRC_DIR "build"

if (-not $OutputDir) {
    $CPP_BUILD_DIR = Join-Path $CPP_SRC_DIR "build"
} else {
    $CPP_BUILD_DIR = $OutputDir
}

if ($EnableLog) {
    $BuildLogDir = Join-Path $ScriptDir "build_logs"
    if (-not (Test-Path $BuildLogDir)) {
        New-Item -ItemType Directory -Path $BuildLogDir -Force | Out-Null
    }

    $Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $LogFile = Join-Path $BuildLogDir "qwen3tts_build_${Timestamp}.log"
}

function log {
    param([string]$m, [string]$c = "White")
    $ts = Get-Date -Format "HH:mm:ss"
    $msg = "[$ts] $m"
    Write-Host $msg -ForegroundColor $c
    if ($EnableLog) {
        Add-Content -Path $LogFile -Value $msg
    }
}

function has { param([string]$c) $null = Get-Command $c -ErrorAction SilentlyContinue; return $? }

function cmake-ok {
    try { $v = & cmake --version 2>$null; return ($v -match "(\d+\.\d+)") } catch { return $false }
}

function best-gen {
    if ($Generator) { return $Generator }
    $gcc = Get-Command "gcc" -ErrorAction SilentlyContinue
    if ($gcc) { return "MinGW Makefiles" }
    if (has ninja) { return "Ninja" }
    $msb = Get-Command "MSBuild.exe" -ErrorAction SilentlyContinue
    if ($msb) { return "Visual Studio 17 2022" }
    throw "No C/C++ compiler toolchain found"
}

function comp-info {
    $i = @{}
    $i.GCC = (Get-Command "gcc" -ErrorAction SilentlyContinue)
    $i.GPP = (Get-Command "g++" -ErrorAction SilentlyContinue)
    if ($i.GCC) { $i.Type = "MinGW" } else { $i.Type = "MSVC" }
    return $i
}

function ggml-ok {
    $libs = @("src\ggml.a", "src\ggml-base.a", "src\ggml-cpu.a")
    if ($EnableVulkan) {
        $libs += "src\ggml-vulkan\ggml-vulkan.a"
    }
    foreach ($l in $libs) {
        if (-not (Test-Path (Join-Path $GGML_BUILD_DIR $l))) { return $false }
    }
    return $true
}

function die {
    param([string]$m, [int]$code = 1)
    log "[FATAL] $m" "Red"
    log "Full log: $LogFile" "Red"
    exit $code
}

function run-cmake {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$ArgList)
    $output = & cmake @ArgList 2>&1
    $ec = $LASTEXITCODE
    if ($output -and $EnableLog) {
        $output | ForEach-Object { Add-Content -Path $LogFile -Value $_ }
    }
    return $ec
}

try {
    log "========================================" "Cyan"
    log "Qwen3-TTS C++ Library Build Start" "Cyan"
    log "========================================" "Cyan"
    log "Source Dir:  $CPP_SRC_DIR"
    log "Build Dir:   $CPP_BUILD_DIR"
    log "Build Type:  $BuildType"
    log "Parallel:    $ParallelJobs"
    log "GGML Dir:    $GGML_BUILD_DIR"
    log "Log File:    $LogFile"
    if ($DllOutputDir) {
        log "DLL Out Dir: $DllOutputDir"
    }
    if ($EnableVulkan) {
        log "Vulkan GPU:  ENABLED" "Green"
    } else {
        log "Vulkan GPU:  DISABLED" "Yellow"
    }
    log "========================================" "Cyan"

    if (-not $SkipGGML) {
        if (-not (ggml-ok)) {
            die "GGML libraries not found. Run build_ggml.ps1 first."
        }
        log "GGML library verification passed" "Green"
    } else {
        log "Skipping GGML library check (-SkipGGML)" "Yellow"
    }

    if (-not (has cmake)) { die "CMake not found" }
    if (-not (cmake-ok)) { die "CMake 3.14+ required" }

    $cmakeVer = & cmake --version 2>$null
    log "CMake version: $cmakeVer" "Green"

    $ci = comp-info
    log "Compiler type: $($ci.Type)" "Green"

    $gen = best-gen

    if ($Clean -and (Test-Path $CPP_BUILD_DIR)) {
        log "Cleaning old build directory..." "Yellow"
        Remove-Item -Recurse -Force $CPP_BUILD_DIR
    }

    if (-not (Test-Path $CPP_BUILD_DIR)) {
        New-Item -ItemType Directory -Path $CPP_BUILD_DIR -Force | Out-Null
        log "Created build directory: $CPP_BUILD_DIR" "Green"
    } else {
        log "Build directory exists, performing incremental build" "Green"
    }

    $bt = $BuildType.ToUpper()
    $cmakeConfigArgs = @(
        "-S", $CPP_SRC_DIR,
        "-B", $CPP_BUILD_DIR,
        "-G", $gen,
        "-DCMAKE_BUILD_TYPE=$bt",
        "-DQWEN3_TTS_TIMING=OFF"
    )

    if ($DllOutputDir) {
        $dllOutAbs = [System.IO.Path]::GetFullPath($DllOutputDir)
        if (-not (Test-Path $dllOutAbs)) {
            New-Item -ItemType Directory -Path $dllOutAbs -Force | Out-Null
        }
        $cmakeConfigArgs += "-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=$dllOutAbs"
        log "DLL runtime output directory: $dllOutAbs" "Green"
    }

    if ($ci.Type -eq "MinGW") {
        $cmakeConfigArgs += "-DCMAKE_C_COMPILER=$($ci.GCC.Source)"
        $cmakeConfigArgs += "-DCMAKE_CXX_COMPILER=$($ci.GPP.Source)"
    }

    log "Running CMake configure (generator: $gen)..." "Yellow"

    $ec = run-cmake @cmakeConfigArgs
    if ($ec -ne 0) {
        die "CMake configure FAILED (exit code: $ec)"
    }

    log "CMake configure completed" "Green"
    log "Running CMake build (parallel: $ParallelJobs)..." "Yellow"

    $buildArgs = @("--build", $CPP_BUILD_DIR, "--config", $bt, "--parallel", $ParallelJobs)

    $ec = run-cmake @buildArgs
    if ($ec -ne 0) {
        die "CMake build FAILED (exit code: $ec)"
    }

    log "Qwen3-TTS C++ build completed" "Green"

    $dlls = Get-ChildItem -Path $CPP_BUILD_DIR -Filter "*.dll" -ErrorAction SilentlyContinue
    $implibs = Get-ChildItem -Path $CPP_BUILD_DIR -Filter "*.dll.a" -ErrorAction SilentlyContinue

    log "Build artifacts:" "Cyan"
    foreach ($f in $dlls) {
        log "  DLL:    $($f.Name) ($([math]::Round($f.Length/1KB,1)) KB)" "White"
    }
    foreach ($f in $implibs) {
        log "  IMPLIB: $($f.Name) ($([math]::Round($f.Length/1KB,1)) KB)" "White"
    }

    if ($DllOutputDir) {
        $dllOutAbs = [System.IO.Path]::GetFullPath($DllOutputDir)
        $targetDlls = Get-ChildItem -Path $dllOutAbs -Filter "qwen3tts.dll" -ErrorAction SilentlyContinue
        foreach ($f in $targetDlls) {
            log "  DLL_OUT: $($f.FullName) ($([math]::Round($f.Length/1KB,1)) KB)" "Green"
        }
    }

    log "========================================" "Cyan"
    log "Qwen3-TTS C++ Library Build Complete!" "Green"
    log "========================================" "Cyan"

    log "Build succeeded!" "Green"
    log "Build type: $BuildType" "Green"
    log "Build dir: $CPP_BUILD_DIR" "Green"
    log "Log file: $LogFile" "Green"
    exit 0

} catch {
    die "Qwen3-TTS C++ build failed: $_"
}