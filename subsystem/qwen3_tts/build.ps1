# build.ps1 - Qwen3_TTS_Lunar Master Build Script
# 3-stage build: GGML -> C++ -> Go
# 默认只编译 C++ 部分（GGML 静态库 + qwen3tts.dll），不编译 Go EXE；
# 需要一并产出 Qwen3_TTS_Lunar.exe 时显式加 -WithGo。
param(
    [ValidateSet("Debug", "Release")]
    [string]$BuildType = "Release",

    [switch]$Clean,

    [switch]$SkipGGML,

    [switch]$SkipCPP,

    [switch]$SkipGo,

    # 显式构建 Go EXE（默认关闭：只编译 CPP 库）
    [switch]$WithGo,

    # 目标平台：由 subsystem/build.ps1 传入。此前本脚本未声明这两个参数，编排器传进来的
    # -TargetOS/-TargetArch 会被 PowerShell 静默收进 $args 丢掉（脚本非高级函数，不报错）。
    # C++ 库（qwen3tts.dll + MinGW 导入库）只能构建 Windows 目标，非 windows 时见下方跳过分支。
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    [switch]$EnableLog,

    [int]$ParallelJobs = $env:NUMBER_OF_PROCESSORS,

    [string]$OutputDir = "",

    [switch]$EnableVulkan = $true,

    [string]$DllOutputDir = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# 载入共享构建辅助（Go 工具链定位）
$repoRoot = $ScriptDir
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")

# C++ 库是 Windows/MinGW 专用产物：subsystem/qwen3_tts/module/generate.go 用 cgo 链接
# cpp/build/libqwen3tts.dll.a（MinGW 导入库）并依赖 -lgomp。
# 因此非 Windows 目标无法在此产出可用的 C++ 库——跳过而不是悄悄产出一个用不上的 DLL。
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

function Build-IconIfNeeded {
    $iconPath = Join-Path $ScriptDir "icon.ico"
    $sysoPath = Join-Path $ScriptDir "icon.syso"

    if (-not (Test-Path $iconPath)) {
        return
    }
    if (Test-Path $sysoPath) {
        return
    }

    Write-BuildLog "[Icon] Compiling icon resource..." "Yellow"
    Push-Location $ScriptDir
    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        throw "rsrc icon compilation failed"
    }
    Pop-Location
    Write-BuildLog "  [OK] icon.syso generated" "Green"
}

Write-BuildLog "========================================" "Cyan"
Write-BuildLog "Qwen3_TTS_Lunar Build Start" "Cyan"
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
if ($WithGo -and -not $SkipGo) {
    Write-BuildLog "Go EXE:      ENABLED (-WithGo)" "Green"
} else {
    Write-BuildLog "Go EXE:      DISABLED (CPP library only)" "Yellow"
}
Write-BuildLog "========================================" "Cyan"

if (-not $CppTargetSupported) {
    Write-BuildLog "" "White"
    Write-BuildLog "[SKIP] TargetOS=$TargetOS : Qwen3-TTS C++ 库是 Windows/MinGW 专用产物" "Yellow"
    Write-BuildLog "       module/generate.go 通过 cgo 链接 cpp/build/libqwen3tts.dll.a + -lgomp，" "DarkGray"
    Write-BuildLog "       无法在 Windows 主机上为该目标产出可用的 C++ 库，故跳过 CPP/Go 阶段。" "DarkGray"
    Write-BuildLog "       （其余子系统的 Go 产物由 subsystem/build.ps1 正常交叉编译）" "DarkGray"
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

if ($WithGo -and -not $SkipGo) {
    # 复用共享定位：PATH -> 常见安装目录 -> %USERPROFILE%\sdk\go*（版本号大者优先）
    $script:GoToolchain = Resolve-Go -Purpose "构建 Qwen3_TTS_Lunar.exe"
    $goVer = & $script:GoToolchain version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $goVer" "Green"
    Write-BuildLog "  Go 路径: $script:GoToolchain" "DarkGray"
} else {
    # 默认只编 CPP 库，不应因为缺少 Go 工具链而失败
    Write-BuildLog "  [SKIP] go not required (CPP library only)" "Yellow"
}

if (Test-CommandExists "gcc") {
    $gccVer = & gcc --version 2>&1 | Select-Object -First 1
    Write-BuildLog "  [OK] $gccVer" "Green"
} else {
    Write-BuildLog "  [FAIL] gcc not found" "Red"
    throw "gcc is required (MinGW-w64 recommended)"
}

Write-BuildLog "[Check] Environment verification complete" "Green"

$goExeOutDir = Join-Path (Resolve-Path (Join-Path $ScriptDir "..\..")) ""
if (-not $DllOutputDir) {
    $DllOutputDir = $goExeOutDir
}
$DllOutputDir = [System.IO.Path]::GetFullPath($DllOutputDir)

$buildScriptArgs = @{
    BuildType = $BuildType
    Clean = $Clean
    ParallelJobs = $ParallelJobs
    # 让 C++ 阶段真正收到目标平台（build_ggml/build_cpp 会据此设置 CMAKE_SYSTEM_NAME）
    TargetOS = $TargetOS
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

if ($DllOutputDir) {
    $buildScriptArgs.DllOutputDir = $DllOutputDir
}

if (-not $SkipGGML) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 1/3 : Build GGML Library" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    $ggmlScript = Join-Path $ScriptDir "build_ggml.ps1"
    Invoke-BuildScript -ScriptPath $ggmlScript -Arguments $buildScriptArgs -StepName "GGML Build"
} else {
    Write-BuildLog "[Stage 1/3] Skipped (-SkipGGML)" "Yellow"
}

if (-not $SkipCPP) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 2/3 : Build Qwen3-TTS C++ Library" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    $cppScript = Join-Path $ScriptDir "build_cpp.ps1"
    Invoke-BuildScript -ScriptPath $cppScript -Arguments $buildScriptArgs -StepName "C++ Build"
} else {
    Write-BuildLog "[Stage 2/3] Skipped (-SkipCPP)" "Yellow"
}

if ($WithGo -and -not $SkipGo) {
    Write-BuildLog "" "White"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "  Stage 3/3 : Build Go Application (-WithGo)" "Cyan"
    Write-BuildLog "============================================================" "Cyan"
    Write-BuildLog "" "White"

    Build-IconIfNeeded

    Write-BuildLog "Running go build..." "Yellow"

    Push-Location $ScriptDir

    $env:CGO_ENABLED = "1"
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_LDFLAGS = "-static-libgcc -static-libstdc++"

    # 用 Resolve-Go 解析出的绝对路径，而不是依赖 PATH（%USERPROFILE%\sdk 布局的 Go 不在 PATH 里）
    $goOutput = (& $script:GoToolchain build -v -o ..\..\Qwen3_TTS_Lunar.exe -ldflags "-s -w" 2>&1 | Out-String)
    $goExitCode = $LASTEXITCODE

    if ($goOutput -and $EnableLog) {
        foreach ($line in ($goOutput -split "`r`n")) {
            if ($line) {
                Add-Content -Path $LogFile -Value $line
            }
        }
    }

    if ($goExitCode -ne 0) {
        Write-BuildLog "Go build FAILED (exit code: $goExitCode)" "Red"
        if ($goOutput) { Write-Host $goOutput -ForegroundColor Red }
        Pop-Location
        throw "Go build failed"
    }

    $exePath = Join-Path $goExeOutDir "Qwen3_TTS_Lunar.exe"
    if (Test-Path $exePath) {
        $exeSize = [math]::Round((Get-Item $exePath).Length / 1MB, 2)
        Write-BuildLog "  [OK] Qwen3_TTS_Lunar.exe ($exeSize MB)" "Green"
    } else {
        Write-BuildLog "  [WARN] Qwen3_TTS_Lunar.exe not found at $exePath" "Yellow"
    }

    Pop-Location
    Write-BuildLog "Go build completed" "Green"
} elseif ($SkipGo) {
    Write-BuildLog "[Stage 3/3] Skipped (-SkipGo)" "Yellow"
} else {
    Write-BuildLog "[Stage 3/3] Skipped - CPP library only by default (add -WithGo to also build the Go EXE)" "Yellow"
}

# ---------- CPP 产物确认 ----------
# 默认（不建 Go）时这一段是唯一的产物反馈，务必保留在 Go 分支之外。
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