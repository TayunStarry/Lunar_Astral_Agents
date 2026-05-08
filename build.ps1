<#
.SYNOPSIS
    Lunar_Astral_Agents 整体构建调度脚本
.DESCRIPTION
    集中执行所有环境检查，然后依次调用各子系统的构建脚本，
    确保环境检查仅执行一次，提升整体构建效率。
.PARAMETER TargetOS
    目标操作系统，可选值：windows, linux, darwin。默认 windows。
.PARAMETER TargetArch
    目标 CPU 架构，可选值：amd64, arm64。默认 amd64。
.EXAMPLE
    .\build.ps1 -TargetOS linux -TargetArch arm64
#>

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"

# ---------- 集中环境检查 ----------
function Check-GoEnvironment {
    Write-Host "[Env] 检查 Go 环境..." -ForegroundColor Cyan
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        throw "未找到 Go。请安装 Go 1.20 或更高版本：https://go.dev/dl/"
    }
    $goVersion = (go version | Select-String -Pattern "go(\d+\.\d+\.\d+)" | ForEach-Object { $_.Matches.Groups[1].Value })
    if (-not $goVersion) {
        throw "无法解析 Go 版本。请确保已安装 Go 1.20+。"
    }
    $minVersion = [version]"1.20.0"
    if ([version]$goVersion -lt $minVersion) {
        throw "Go 版本过低 ($goVersion)，需要 >= 1.20。请升级：https://go.dev/dl/"
    }
    Write-Host "  ✓ Go $goVersion" -ForegroundColor Green
}

function Check-GCC {
    Write-Host "[Env] 检查 GCC (CGO 需要)..." -ForegroundColor Cyan
    $gccCmd = Get-Command gcc -ErrorAction SilentlyContinue
    if (-not $gccCmd) {
        throw "未找到 GCC。CGO 需要 GCC，Windows 下请安装 MinGW-w64 或 TDM-GCC：https://www.mingw-w64.org/"
    }
    $gccOutput = & gcc --version 2>&1 | Select-Object -First 1
    Write-Host "  ✓ GCC: $gccOutput" -ForegroundColor Green
}

function Check-NodeEnvironment {
    Write-Host "[Env] 检查 Node.js..." -ForegroundColor Cyan
    $nodeCmd = Get-Command node -ErrorAction SilentlyContinue
    if (-not $nodeCmd) {
        throw "未找到 Node.js。请安装 Node.js 18+：https://nodejs.org/"
    }
    $nodeVersion = & node --version
    Write-Host "  ✓ Node.js $nodeVersion" -ForegroundColor Green
}

function Check-NPM {
    Write-Host "[Env] 检查 npm..." -ForegroundColor Cyan
    $npmCmd = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $npmCmd) {
        throw "未找到 npm（通常随 Node.js 安装）。"
    }
    $npmVersion = & npm --version
    Write-Host "  ✓ npm v$npmVersion" -ForegroundColor Green
}

function Check-RsrcTool {
    Write-Host "[Env] 检查 rsrc 工具..." -ForegroundColor Cyan
    $rsrcCmd = Get-Command rsrc -ErrorAction SilentlyContinue
    if (-not $rsrcCmd) {
        Write-Host "  rsrc 未安装，尝试自动安装..." -ForegroundColor Yellow
        try {
            & go install github.com/akavel/rsrc@latest 2>&1 | Out-Host
            if ($LASTEXITCODE -ne 0) { throw }
            $goPath = & go env GOPATH
            $env:Path = "$goPath\bin;$env:Path"
        }
        catch {
            throw "安装 rsrc 失败: $_。请手动执行：go install github.com/akavel/rsrc@latest"
        }
        $rsrcCmd = Get-Command rsrc -ErrorAction SilentlyContinue
        if (-not $rsrcCmd) {
            throw "安装 rsrc 后仍未找到命令，请确保 %GOPATH%\bin 在 PATH 中。"
        }
    }
    Write-Host "  ✓ rsrc 工具可用" -ForegroundColor Green
}

# ---------- 主调度流程 ----------
try {
    Write-Host "`n========== Lunar_Astral_Agents 整体构建 ==========" -ForegroundColor Magenta
    Write-Host "目标平台: $TargetOS / $TargetArch" -ForegroundColor Magenta
    Write-Host "==============================================`n" -ForegroundColor Magenta

    # 集中执行所有环境检查
    Write-Host "---------- 集中环境检查 ----------" -ForegroundColor Cyan
    Check-GoEnvironment
    Check-GCC
    Check-NodeEnvironment
    Check-NPM
    Check-RsrcTool
    Write-Host "✓ 所有环境检查通过`n" -ForegroundColor Green

    # 构建LunarCore（传递 -SkipCheck 参数）
    Write-Host "---------- 构建 LunarCore ----------" -ForegroundColor Cyan
    Set-Location -Path './LunarCore'
    & .\build.ps1 -TargetOS $TargetOS -TargetArch $TargetArch -SkipCheck
    if ($LASTEXITCODE -ne 0) { throw "LunarCore 构建失败" }
    Set-Location -Path '../'
    Write-Host ""

    # 构建 Crystal_Astral（传递 -SkipCheck 参数）
    Write-Host "---------- 构建 Crystal_Astral ----------" -ForegroundColor Cyan
    Set-Location -Path './subsystem\crystal_astral'
    & .\build.ps1 -TargetOS $TargetOS -TargetArch $TargetArch -SkipCheck
    if ($LASTEXITCODE -ne 0) { throw "Crystal_Astral 构建失败" }
    Set-Location -Path '../../'
    Write-Host ""

    # 构建 bridge_adapter（传递 -SkipCheck 参数）
    Write-Host "---------- 构建 bridge_adapter ----------" -ForegroundColor Cyan
    Set-Location -Path './subsystem\bridge_adapter'
    & .\build.ps1 -TargetOS $TargetOS -TargetArch $TargetArch -SkipCheck
    if ($LASTEXITCODE -ne 0) { throw "bridge_adapter 构建失败" }
    Set-Location -Path '../../'
    Write-Host ""

    # 构建 project_archiving（传递 -SkipCheck 参数）
    Write-Host "---------- 构建 project_archiving ----------" -ForegroundColor Cyan
    Set-Location -Path './subsystem\project_archiving'
    & .\build.ps1 -TargetOS $TargetOS -TargetArch $TargetArch -SkipCheck
    if ($LASTEXITCODE -ne 0) { throw "project_archiving 构建失败" }
    Set-Location -Path '../../'
    Write-Host ""

    Write-Host "==============================================" -ForegroundColor Magenta
    Write-Host "✓ 所有模块构建成功！" -ForegroundColor Green
    Write-Host "==============================================`n" -ForegroundColor Magenta
}
catch {
    Write-Host "`n[ERROR] 构建失败: $_" -ForegroundColor Red
    exit 1
}
