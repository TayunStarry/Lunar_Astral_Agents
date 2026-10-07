# build.ps1 - QwenTTS Engine Build Script (lunar_astral 内置引擎)
# 2-stage build: GGML -> C++
# 产出 qwen3tts.dll（含 GGML 静态库），默认输出到仓库根的
# local_data\models\Qwen3-TTS（与 GGUF 模型同目录），由 Lunar_Astral.exe
# 运行时经 LoadLibraryExW + GetProcAddress 动态加载，不再是独立子系统 EXE。
param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [switch]$Clean,

    [switch]$SkipGGML,

    [switch]$SkipCPP,

    # 目标平台：C++ 库（qwen3tts.dll）只能构建 Windows 目标，非 windows 时跳过。
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    [switch]$EnableLog,

    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,

    [string]$OutputDir = "",

    [switch]$EnableVulkan = $true,

    # DLL 输出目录，默认 <仓库根>\local_data\models\Qwen3-TTS
    [string]$DllOutputDir = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# 载入共享构建辅助（Go 工具链定位等）
$repoRoot = $ScriptDir
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")

# C++ 库是 Windows/MinGW 专用产物：Lunar_Astral.exe 运行时动态加载该 DLL。
# 因此非 Windows 目标无法产出可用的 C++ 库——跳过而不是悄悄产出一个用不上的 DLL。
$CppTargetSupported = ($TargetOS -eq "windows")

if ($EnableLog) {
    $BuildLogDir = Join-Path $ScriptDir "build_logs"
    if (-not (Test-Path $BuildLogDir)) {
        New-Item -ItemType Directory -Path $BuildLogDir -Force | Out-Null
    }

    $Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
    $LogFile = Join-Path $BuildLogDir "build_${Timestamp}.log"
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

function Invoke-BuildScript {
    param(
        [string]$ScriptPath,
        [hashtable]$Arguments,
        [string]$StepName
    )

    $argList = @()
    foreach ($key in $Arguments.Keys) {
        $val = $Arguments[$key]
        if ($val -is [switch]) {
            if ($val) { $argList += "-$key" }
        } else {
            $argList += "-$key"
            $argList += "$val"
        }
    }

    Write-BuildLog ">> Calling: $ScriptPath $($argList -join ' ')" "DarkGray"

    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $ScriptPath @argList
    if ($LASTEXITCODE -ne 0) {
        throw "$StepName failed (exit code: $LASTEXITCODE)"
    }
}

# 默认把 DLL 与 GGUF 模型放在一起：local_data\models\Qwen3-TTS
if (-not $DllOutputDir) {
    $DllOutputDir = Join-Path $repoRoot "local_data\models\Qwen3-TTS"
}
$DllOutputDir = [System.IO.Path]::GetFullPath($DllOutputDir)

Write-BuildLog "========================================" "Cyan"
Write-BuildLog "QwenTTS Engine Build Start" "Cyan"
Write-BuildLog "========================================" "Cyan"
Write-BuildLog "Build Type: $BuildType" "White"
if ($EnableLog) {
    Write-BuildLog "Log File:   $LogFile" "White"
}
Write-BuildLog "DLL Out Dir: $DllOutputDir" "White"
if ($EnableVulkan) {
    Write-BuildLog "Vulkan GPU:  ENABLED" "Green"
} else {
    Write-BuildLog "Vulkan GPU:  DISABLED" "Yellow"
}
Write-BuildLog "========================================" "Cyan"

if (-not $CppTargetSupported) {
    Write-BuildLog "" "White"
    Write-BuildLog "[SKIP] TargetOS=$TargetOS : Qwen3-TTS C++ 库是 Windows/MinGW 专用产物" "Yellow"
    Write-BuildLog "       qwen3tts.dll 仅能构建 Windows 目标，故跳过 CPP 阶段。" "DarkGray"
    Write-BuildLog "" "White"
    exit 0
}

Write-BuildLog "[Check] Verifying build environment..." "Yellow"

if (Test-CommandExists "cmake") {
    $cmakeVer = & cmake --version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $cmakeVer" "Green"
} else {
    Write-BuildLog "  [FAIL] cmake not found" "Red"
    throw "cmake is required"
}

if (Test-CommandExists "gcc") {
    $gccVer = & gcc --version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $gccVer" "Green"
} else {
    Write-BuildLog "  [FAIL] gcc not found" "Red"
    throw "gcc is required (MinGW-w64 recommended)"
}

Write-BuildLog "[Check] Environment verification complete" "Green"

$buildScriptArgs = @{
    BuildType = $BuildType
    Clean = $Clean
    ParallelJobs = $ParallelJobs
    # 让 C++ 阶段真正收到目标平台（build_ggml/build_cpp 会据此设置 CMAKE_SYSTEM_NAME）
    TargetOS = $TargetOS
    DllOutputDir = $DllOutputDir
}

if ($OutputDir) {
    $buildScriptArgs.OutputDir = $OutputDir
}

if ($EnableLog) {
    $buildScriptArgs.EnableLog = $EnableLog
}

if ($EnableVulkan) {
    $buildScriptArgs.EnableVulkan = $EnableVulkan
}

if (-not $SkipGGML) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 1/2 : Build GGML Library" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    $ggmlScript = Join-Path $ScriptDir "build_ggml.ps1"
    Invoke-BuildScript -ScriptPath $ggmlScript -Arguments $buildScriptArgs -StepName "GGML Build"
} else {
    Write-BuildLog "[Stage 1/2] Skipped (-SkipGGML)" "Yellow"
}

if (-not $SkipCPP) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 2/2 : Build Qwen3-TTS C++ Library" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    $cppScript = Join-Path $ScriptDir "build_cpp.ps1"
    Invoke-BuildScript -ScriptPath $cppScript -Arguments $buildScriptArgs -StepName "C++ Build"
} else {
    Write-BuildLog "[Stage 2/2] Skipped (-SkipCPP)" "Yellow"
}

# ---------- CPP 产物确认 ----------
Write-BuildLog "" "White"
Write-BuildLog "--- CPP artifacts ---" "Cyan"
$targetDll = Join-Path $DllOutputDir "qwen3tts.dll"
if (Test-Path $targetDll) {
    $dllSize = [math]::Round((Get-Item $targetDll).Length / 1MB, 2)
    Write-BuildLog "  [OK] qwen3tts.dll -> $targetDll ($dllSize MB)" "Green"
} else {
    Write-BuildLog "  [WARN] qwen3tts.dll not found at $targetDll" "Yellow"
}

Write-BuildLog "" "White"
Write-BuildLog "============================================================" "Green"
Write-BuildLog "  BUILD SUCCESSFUL!" "Green"
Write-BuildLog "============================================================" "Green"
if ($EnableLog) {
    Write-BuildLog "Log file: $LogFile" "White"
}
exit 0
