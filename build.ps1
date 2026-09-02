# Lunar Astral Agents - 统一构建脚本
# 负责环境变量检查、依赖验证和统一调度编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"
$ScriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

# ---------- 环境检查函数 ----------
function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Check-GoEnvironment {
    Write-Host "[检查] Go 编程环境..." -ForegroundColor Cyan

    if (-not (Test-CommandExists "go")) {
        throw "未找到 Go 环境，请安装 Go (https://golang.org/dl/)"
    }

    $goVersion = go version 2>&1
    Write-Host "  OK $goVersion" -ForegroundColor Green

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
        [string]$Name
    )

    Write-Host ""
    Write-Host "========== 构建 $Name ==========" -ForegroundColor Magenta

    $buildScript = Join-Path $Path "build.ps1"

    if (-not (Test-Path $buildScript)) {
        throw "未找到构建脚本: $buildScript"
    }

    $originalLocation = Get-Location
    Set-Location -Path $Path

    try {
        & $buildScript -TargetOS $TargetOS -TargetArch $TargetArch
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

    Invoke-Build -Path "$ScriptRoot\subsystem\qwen3_tts" -Name "Qwen3 TTS"
    Invoke-Build -Path "$ScriptRoot\subsystem\qwen_asr" -Name "Qwen ASR"
    Invoke-Build -Path "$ScriptRoot\subsystem\environment_repair" -Name "Environment Repair"
    Invoke-Build -Path "$ScriptRoot\lunar_astral" -Name "Luna Astral"
    Invoke-Build -Path "$ScriptRoot\crystal_astral" -Name "Crystal Astral"

    Write-Host ""
    Write-Host "  全部构建成功完成！" -ForegroundColor Green
    Write-Host ""
}
catch {
    Write-Host ""
    Write-Host "[ERROR] 构建失败: $_" -ForegroundColor Red
    exit 1
}
