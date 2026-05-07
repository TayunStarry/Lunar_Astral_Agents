<#
.SYNOPSIS
    Crystal_Astral 跨平台构建脚本（本地模块友好版）
.DESCRIPTION
    完全跳过哈希校验（go mod verify），仅验证 Go 命令可用和本地替换路径存在，
    随后直接编译。适合所有依赖通过 replace 指向本地目录的项目。
.PARAMETER TargetOS
    目标操作系统，可选值：windows, linux, darwin。默认 windows。
.PARAMETER TargetArch
    目标 CPU 架构，可选值：amd64, arm64。默认 amd64。
.PARAMETER SkipCheck
    跳过所有前置检查，直接编译（当环境已由外部保证时使用）。
.EXAMPLE
    .\build.ps1 -TargetOS darwin -TargetArch arm64
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

# ---------- 轻量 Go 环境验证 ----------
function Test-GoCommand {
    Write-Host "[Verify] 检查 Go 命令是否可用..." -ForegroundColor Cyan
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        throw "未找到 Go 命令。请确保 Go 已安装并加入 PATH。"
    }
    Write-Host "  ✓ Go 命令已就绪" -ForegroundColor Green
}

# ---------- 本地模块路径验证（不调用 go mod verify）----------
function Test-LocalReplacements {
    Write-Host "[Local] 检查本地模块路径..." -ForegroundColor Cyan
    if (-not (Test-Path "go.mod")) {
        throw "未找到 go.mod，请在项目根目录运行此脚本。"
    }

    $modContent = Get-Content "go.mod" -Raw
    # 匹配 replace 指令中的本地路径（相对/绝对）
    $pattern = 'replace\s+\S+\s+=>\s+(\.\.?[\\/][^\s]+|[a-zA-Z]:[\\/][^\s]+|/[^\s]+)'
    $matches = [regex]::Matches($modContent, $pattern)

    $modDir = Split-Path -Path (Resolve-Path "go.mod") -Parent
    foreach ($m in $matches) {
        $localPath = $m.Groups[1].Value
        $fullPath = [System.IO.Path]::GetFullPath([System.IO.Path]::Combine($modDir, $localPath))
        if (-not (Test-Path $fullPath)) {
            throw "本地模块路径不存在: '$localPath' (解析为: $fullPath)。请检查 go.mod 的 replace 指令。"
        }
        # 可选：检查路径下是否有 go.mod，确认是有效 Go 模块
        if (-not (Test-Path (Join-Path $fullPath "go.mod"))) {
            Write-Warning "本地模块路径 '$fullPath' 下未找到 go.mod，编译可能失败。"
        }
    }
    Write-Host "  ✓ 所有本地模块路径存在" -ForegroundColor Green
}

# ---------- 图标资源处理（存在性检查）----------
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
function Build-CrystalAstral {
    Write-Host "`n========== 构建 Crystal_Astral ==========" -ForegroundColor Magenta
    Write-Host "目标平台: $TargetOS / $TargetArch" -ForegroundColor Magenta

    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    # 图标（Windows）
    Build-IconIfNeeded

    # 构建参数
    $ldflags = "-s -w"
    if ($TargetOS -eq "windows") {
        $ldflags += " -H windowsgui"
    }

    $binaryName = "Crystal_Astral"
    if ($TargetOS -eq "windows") { $binaryName += ".exe" }
    # 根据实际需要调整输出路径，此处假设脚本位于 subsystem/crystal_astral 下
    $outputPath = "..\..\$binaryName"

    Write-Host "[Build] 编译 Go 二进制（跳过哈希校验）..." -ForegroundColor Cyan
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
        Test-GoCommand
        Test-LocalReplacements   # 仅检查路径，不执行任何 go mod verify
    }
    else {
        Write-Host "[Skip] 跳过所有检查，直接编译。" -ForegroundColor Yellow
    }
    Build-CrystalAstral
}
catch {
    Write-Host "`n[ERROR] 构建失败: $_" -ForegroundColor Red
    exit 1
}