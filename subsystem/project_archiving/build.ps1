<#
.SYNOPSIS
    project_archiving 构建脚本
.DESCRIPTION
    编译图标并构建 Go 可执行文件。支持独立编译，也可被主调度脚本调用。
.PARAMETER TargetOS
    目标操作系统，可选值：windows, linux, darwin。默认 windows。
.PARAMETER TargetArch
    目标 CPU 架构，可选值：amd64, arm64。默认 amd64。
.PARAMETER SkipCheck
    跳过环境检查（当环境已由外部保证时使用）。
.EXAMPLE
    .\build.ps1 -TargetOS linux -TargetArch arm64
.EXAMPLE
    .\build.ps1 -SkipCheck
#>

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",
    [switch]$SkipCheck
)

$ErrorActionPreference = "Stop"

# ---------- 环境检查（仅在独立编译时执行）----------
function Test-GoCommand {
    Write-Host "[Env] 检查 Go 命令是否可用..." -ForegroundColor Cyan
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        throw "未找到 Go 命令。请确保 Go 已安装并加入 PATH。"
    }
    Write-Host "  ✓ Go 命令已就绪" -ForegroundColor Green
}

# ---------- 图标资源处理 ----------
function Build-IconIfNeeded {
    if ($TargetOS -ne "windows" -or -not (Test-Path "icon.ico")) {
        return
    }

    if (Test-Path "icon.syso") {
        Write-Host "[Icon] icon.syso 已存在，跳过图标编译。" -ForegroundColor DarkGray
        return
    }

    Write-Host "[Icon] 编译图标资源..." -ForegroundColor Cyan
    $rsrcCmd = Get-Command rsrc -ErrorAction SilentlyContinue
    if (-not $rsrcCmd) {
        throw "未找到 rsrc 工具，且 icon.syso 不存在。请先通过前置脚本安装 rsrc，或手动执行：go install github.com/akavel/rsrc@latest"
    }
    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) { throw "rsrc 执行失败。" }
    Write-Host "  ✓ 图标资源生成完毕" -ForegroundColor Green
}

# ---------- 构建主流程 ----------
function Build-ProjectArchiving {
    Write-Host "`n========== 构建 project_archiving ==========" -ForegroundColor Magenta
    Write-Host "目标平台: $TargetOS / $TargetArch" -ForegroundColor Magenta

    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    Build-IconIfNeeded

    $ldflags = "-s -w"
    if ($TargetOS -eq "windows") {
        $ldflags += " -H windowsgui"
    }

    $binaryName = "project_archiving"
    if ($TargetOS -eq "windows") { $binaryName += ".exe" }
    $outputPath = "..\..\$binaryName"

    Write-Host "[Build] 编译 Go 二进制..." -ForegroundColor Cyan
    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Go build 失败。" }

    Write-Host "✓ 构建成功: $outputPath" -ForegroundColor Green
    Write-Host "========== 完成 ==========`n" -ForegroundColor Magenta
}

# ---------- 入口 ----------
try {
    if (-not $SkipCheck) {
        Write-Host "[project_archiving] 独立编译模式，执行环境检查..." -ForegroundColor Cyan
        Test-GoCommand
    }
    else {
        Write-Host "[project_archiving] 由主调度脚本调用，跳过环境检查。" -ForegroundColor Yellow
    }
    Build-ProjectArchiving
}
catch {
    Write-Host "`n[ERROR] 构建失败: $_" -ForegroundColor Red
    exit 1
}
