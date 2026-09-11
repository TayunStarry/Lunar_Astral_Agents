# Lunar Astral Agents - 统一构建脚本
#
# 注意：Qwen3 TTS 默认只编译 CPP 库（GGML + qwen3tts.dll），不编译它的 Go EXE；
# 需要一并产出 Qwen3_TTS_Lunar.exe 时加 -WithGo。
# 其余子系统（environment_repair / lunar_astral / crystal_astral / ltp9_keygen）
# 没有 CPP 库，仍照常构建各自的 Go EXE。
param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    # 透传给 qwen3_tts：额外构建 Qwen3_TTS_Lunar.exe（默认关闭）
    [switch]$WithGo
)

$ErrorActionPreference = "Stop"
$ScriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

# 载入共享构建辅助（Go 工具链定位）
$repoRoot = $ScriptRoot
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")

# ---------- 环境检查函数 ----------
function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Check-GoEnvironment {
    Write-Host "[检查] Go 编程环境..." -ForegroundColor Cyan

    # 复用共享定位：PATH -> 常见安装目录 -> %USERPROFILE%\sdk\go*（取最新版本）
    # 注意 PATH 里通常没有 %USERPROFILE%\sdk 布局的 Go，只查 PATH 会误报"未安装"
    $script:GoToolchain = Resolve-Go -Purpose "统一构建"
    $goVersion = & $script:GoToolchain version 2>&1
    Write-Host "  OK $goVersion" -ForegroundColor Green
    Write-Host "  Go 路径: $script:GoToolchain" -ForegroundColor DarkGray

    if ($TargetOS -eq "windows") {
        if (-not (Test-CommandExists "gcc")) {
            throw "未找到 GCC 编译器，CGO 支持需要 GCC (请安装 MinGW-w64 或 TDM-GCC)"
        }
        $gccVersion = gcc --version 2>&1 | Select-Object -First 1
        Write-Host "  OK $gccVersion" -ForegroundColor Green
    }
}

function Check-NodeEnvironment {
    Write-Host "[检查] Node.js 运行时..." -ForegroundColor Cyan

    if (-not (Test-CommandExists "node")) {
        throw "未找到 Node.js 运行时，请安装 Node.js (https://nodejs.org/)"
    }

    $nodeVersion = node --version 2>&1
    Write-Host "  OK Node.js $nodeVersion" -ForegroundColor Green
}

function Check-NpmEnvironment {
    Write-Host "[检查] npm 包管理器..." -ForegroundColor Cyan

    if (-not (Test-CommandExists "npm")) {
        throw "未找到 npm 包管理器，请确保已安装 npm"
    }

    $npmVersion = npm --version 2>&1
    Write-Host "  OK npm $npmVersion" -ForegroundColor Green
}

function Check-RsrcTool {
    if ($TargetOS -eq "windows") {
        Write-Host "[检查] rsrc 图标编译工具..." -ForegroundColor Cyan

        if (-not (Test-CommandExists "rsrc")) {
            throw "未找到 rsrc 工具，请安装 (go install github.com/akavel/rsrc@latest)"
        }

        Write-Host "  OK rsrc 工具已安装" -ForegroundColor Green
    }
}

# ---------- 统一构建函数 ----------
function Invoke-Build {
    param(
        [string]$Path,
        [string]$Name,
        [hashtable]$ExtraArgs = @{}
    )

    Write-Host ""
    Write-Host "========== 构建 $Name ==========" -ForegroundColor Magenta

    $buildScript = Join-Path $Path "build.ps1"

    if (-not (Test-Path $buildScript)) {
        throw "未找到构建脚本: $buildScript"
    }

    $originalLocation = Get-Location
    Set-Location -Path $Path

    # TargetOS/TargetArch 为通用参数；ExtraArgs 用于透传子系统专属开关
    $splat = @{ TargetOS = $TargetOS; TargetArch = $TargetArch }
    foreach ($key in $ExtraArgs.Keys) { $splat[$key] = $ExtraArgs[$key] }

    try {
        & $buildScript @splat
    }
    catch {
        Set-Location -Path $originalLocation
        throw "构建 $Name 失败: $_"
    }

    Set-Location -Path $originalLocation
    Write-Host "========== $Name 构建完成 ==========" -ForegroundColor Magenta
    Write-Host ""
}

# ---------- 构建主流程 ----------
try {
    Write-Host ""
    Write-Host "  Lunar Astral Agents - 统一构建系统" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "目标平台: $TargetOS / $TargetArch" -ForegroundColor Yellow
    Write-Host ""

    # 阶段 1: 环境检查
    Write-Host "--- 阶段 1: 环境与依赖检查 ---" -ForegroundColor Yellow

    Check-GoEnvironment
    Check-NodeEnvironment
    Check-NpmEnvironment
    Check-RsrcTool

    Write-Host ""
    Write-Host "[OK] 所有环境检查通过！" -ForegroundColor Green
    Write-Host ""

    # 阶段 2: 编译项目
    Write-Host "--- 阶段 2: 项目编译 ---" -ForegroundColor Yellow

    # Qwen3 TTS 默认只出 CPP 库（GGML + qwen3tts.dll）；-WithGo 时才连 Go EXE 一起构建
    $ttsArgs = @{}
    if ($WithGo) { $ttsArgs['WithGo'] = $true }
    Invoke-Build -Path "$ScriptRoot\qwen3_tts" -Name "Qwen3 TTS (CPP library)" -ExtraArgs $ttsArgs
    Invoke-Build -Path "$ScriptRoot\environment_repair" -Name "Environment Repair"
    Invoke-Build -Path "$ScriptRoot\..\lunar_astral" -Name "Luna Astral"
    Invoke-Build -Path "$ScriptRoot\..\crystal_astral" -Name "Crystal Astral"
    Invoke-Build -Path "$ScriptRoot\ltp9_keygen" -Name "LTP9 Keygen"

    Write-Host ""
    Write-Host "  全部构建成功完成！" -ForegroundColor Green
    Write-Host ""
}
catch {
    Write-Host ""
    Write-Host "[ERROR] 构建失败: $_" -ForegroundColor Red
    exit 1
}
