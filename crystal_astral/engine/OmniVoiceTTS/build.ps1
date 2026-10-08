﻿# build.ps1 - OmniVoiceTTS Engine Build Script (crystal_astral 内置引擎)
# 产出 omnivoice.dll（GGML 推理 + OV 公共 ABI，MinGW + Vulkan），
# 默认输出到仓库根的 local_data\models\OmniVoice（与 GGUF 模型同目录），
# 由 Crystal_Astral.exe 运行时经 LoadLibraryExW + GetProcAddress 动态加载。
param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [switch]$Clean,

    # 目标平台：C++ 库（omnivoice.dll）只能构建 Windows 目标，非 windows 时跳过。
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,

    [string]$OutputDir = "",

    # DLL 输出目录，默认 <仓库根>\local_data\models\OmniVoice
    [string]$DllOutputDir = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# 定位仓库根（subsystem\build_common.ps1 所在处）
$repoRoot = $ScriptDir
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")

$CppSourceDir = Join-Path $ScriptDir "cpp"
if ($OutputDir) {
    $BuildDir = $OutputDir
} else {
    $BuildDir = Join-Path $CppSourceDir "build"
}

if (-not $DllOutputDir) {
    $DllOutputDir = Join-Path $repoRoot "local_data\models\OmniVoice"
}
$DllOutputDir = [System.IO.Path]::GetFullPath($DllOutputDir)

function log {
    param([string]$m, [string]$c = "White")
    $ts = Get-Date -Format "HH:mm:ss"
    Write-Host "[$ts] $m" -ForegroundColor $c
}

function has { param([string]$c) $null = Get-Command $c -ErrorAction SilentlyContinue; return $? }

# ---- 目标平台校验：本库只产出 Windows DLL ----
if ($TargetOS -ne "windows") {
    log "[SKIP] TargetOS=$TargetOS : omnivoice.dll 仅支持 Windows/MinGW 目标" "Yellow"
    exit 0
}

log "========================================" "Cyan"
log "OmniVoiceTTS Engine Build Start" "Cyan"
log "========================================" "Cyan"
log "Source Dir:  $CppSourceDir"
log "Build Dir:   $BuildDir"
log "Build Type:  $BuildType"
log "Parallel:    $ParallelJobs"
log "DLL Out Dir: $DllOutputDir"
log "========================================" "Cyan"

if (-not (has cmake)) { throw "cmake not found" }
if (-not (has gcc)) { throw "gcc (MinGW-w64) not found" }
if (-not (has "g++")) { throw "g++ (MinGW-w64) not found" }

# Vulkan SDK 探测：ggml-vulkan 需要 glslc 编译着色器
$vulkanSdk = $env:VULKAN_SDK
if (-not ($vulkanSdk -and (Test-Path $vulkanSdk))) {
    foreach ($p in @("C:\VulkanSDK", "${env:ProgramFiles}\VulkanSDK", "${env:LOCALAPPDATA}\VulkanSDK")) {
        if ($p -and (Test-Path $p)) {
            $vulkanSdk = (Get-ChildItem $p -Directory | Sort-Object Name -Descending | Select-Object -First 1).FullName
            break
        }
    }
}
if ($vulkanSdk) {
    $glslcDir = Join-Path $vulkanSdk "Bin"
    if (Test-Path (Join-Path $glslcDir "glslc.exe")) {
        $env:PATH = "$glslcDir;$env:PATH"
        log "Vulkan SDK: $vulkanSdk (glslc READY)" "Green"
    } else {
        log "Vulkan SDK: $vulkanSdk (未找到 glslc.exe，Vulkan 后端将不可用)" "Yellow"
    }
} else {
    log "Vulkan SDK: 未找到，将回退 CPU 后端" "Yellow"
}

# 失效缓存检测：CMakeCache.txt 记录创建时的绝对路径，目录迁移后自动清理
$cacheFile = Join-Path $BuildDir "CMakeCache.txt"
if (Test-Path $cacheFile) {
    foreach ($line in (Get-Content $cacheFile -TotalCount 20)) {
        if ($line -match "^#+ For build in directory: (.+)$") {
            $recorded = $Matches[1].Trim().Replace('/', '\')
            if ($recorded -ne $BuildDir) {
                log "检测到失效的 CMake 缓存（创建于 $recorded），自动清理后重新配置..." "Yellow"
                Remove-Item -Recurse -Force $BuildDir
            }
            break
        }
    }
}

if ($Clean -and (Test-Path $BuildDir)) {
    log "Cleaning old build directory..." "Yellow"
    Remove-Item -Recurse -Force $BuildDir
}

if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
    log "Created build directory: $BuildDir" "Green"
} else {
    log "Build directory exists, performing incremental build" "Green"
}

$gcc = (Get-Command gcc).Source
$gpp = (Get-Command "g++").Source
$cmakeArgs = @(
    "-S", $CppSourceDir,
    "-B", $BuildDir,
    "-G", "MinGW Makefiles",
    "-DCMAKE_BUILD_TYPE=$BuildType",
    "-DGGML_VULKAN=ON",
    "-DOMNIVOICE_SHARED=ON",
    # 只产出共享库，跳过 CLI 工具与 tts-server 的编译
    "-DGGML_CUDA=OFF",
    "-DGGML_METAL=OFF",
    "-DGGML_BLAS=OFF",
    "-DGGML_BACKEND_DL=OFF",
    "-DGGML_NATIVE=ON",
    # MinGW 共享库：libgcc/libstdc++/winpthread 静态链入，libgomp 保持动态
    "-DCMAKE_SHARED_LINKER_FLAGS=-static-libgcc -static-libstdc++ -Wl,-Bstatic -lwinpthread -Wl,-Bdynamic",
    "-DCMAKE_C_COMPILER=$gcc",
    "-DCMAKE_CXX_COMPILER=$gpp"
)

log "Running CMake configure..." "Yellow"
$output = & cmake @cmakeArgs 2>&1
$configureExit = $LASTEXITCODE
if ($configureExit -ne 0) {
    $output | Select-Object -Last 40 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkRed }
    throw "CMake configure FAILED (exit code: $configureExit)"
}
log "CMake configure completed" "Green"

log "Running CMake build (target: omnivoice, parallel: $ParallelJobs)..." "Yellow"
$output = & cmake --build $BuildDir --target omnivoice --parallel $ParallelJobs 2>&1
$buildExit = $LASTEXITCODE
if ($buildExit -ne 0) {
    $output | Select-Object -Last 60 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkRed }
    throw "CMake build FAILED (exit code: $buildExit)"
}
log "CMake build completed" "Green"

# ---- 产物确认与就位 ----
# MinGW 会给共享库自动加 lib 前缀（libomnivoice.dll），MSVC 则为 omnivoice.dll
$dllSource = Join-Path $BuildDir "libomnivoice.dll"
if (-not (Test-Path $dllSource)) {
    $dllSource = Join-Path $BuildDir "omnivoice.dll"
}
if (-not (Test-Path $dllSource)) {
    throw "omnivoice.dll not found at $BuildDir"
}

New-Item -ItemType Directory -Path $DllOutputDir -Force | Out-Null
Copy-Item $dllSource (Join-Path $DllOutputDir "omnivoice.dll") -Force
$dllSize = [math]::Round((Get-Item $dllSource).Length / 1MB, 2)
log "[OK] omnivoice.dll -> $DllOutputDir ($dllSize MB)" "Green"

# ---- 依赖 DLL 就位（LOAD_WITH_ALTERED_SEARCH_PATH 从 DLL 同目录解析）----
#   - vulkan-1.dll        Vulkan 加载器（优先 Vulkan SDK，其次系统副本）
#   - libgomp-1.dll       OpenMP 运行时（MinGW，libgomp 无法静态链接）
#   - libwinpthread-1.dll MinGW pthread
$gccBinDir = Split-Path -Parent $gcc
$vulkanCandidates = @()
if ($vulkanSdk) { $vulkanCandidates += (Join-Path $vulkanSdk "Bin\vulkan-1.dll") }
$vulkanCandidates += (Join-Path $env:SystemRoot "System32\vulkan-1.dll")
$depSources = @{
    "libgomp-1.dll"       = @((Join-Path $gccBinDir "libgomp-1.dll"))
    "libwinpthread-1.dll" = @((Join-Path $gccBinDir "libwinpthread-1.dll"))
    "vulkan-1.dll"        = $vulkanCandidates
}
log "--- dependency DLLs ---" "Cyan"
foreach ($depName in @("vulkan-1.dll", "libgomp-1.dll", "libwinpthread-1.dll")) {
    $depDest = Join-Path $DllOutputDir $depName
    if (Test-Path $depDest) {
        log "  [SKIP] $depName (已存在)" "DarkGray"
        continue
    }
    $depSrc = $depSources[$depName] | Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
    if (-not $depSrc) {
        log "  [WARN] 未找到 $depName 的来源副本，运行时 GPU/多线程可能降级" "Yellow"
        continue
    }
    Copy-Item $depSrc $depDest -Force
    log "  [OK] $depName <- $depSrc" "Green"
}

log "============================================================" "Green"
log "BUILD SUCCESSFUL!" "Green"
log "============================================================" "Green"
exit 0
