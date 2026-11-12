# Update Applier - 编译脚本
# 将「更新器」编译为独立可执行文件，输出到仓库根目录（与主程序同一层）
#
# 更新器自行承担打包上传、版本查询与下载安装：
#   不带参数启动进入 TUI（总览 / 版本列表 / 下载安装 / 打包上传 / 日志）。
#   运维参数：-pack 仅打包、-publish 打包并上传、-headless 走命令行、-target 指定目标目录。
# 仓库根目录的 build.ps1 会依次调用主程序与更新器的构建脚本，
# 因此一次构建即可在仓库根目录同时得到 Lunar_Astral_Agents.exe 与 Lunar_Update_Applier.exe。
#
# 用法: .\build.ps1 [-TargetOS windows|linux|darwin] [-TargetArch amd64|arm64]

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"
$ScriptDir = $PSScriptRoot

# 载入共享构建辅助（Go 工具链定位）
$repoRoot = $ScriptDir
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")
$Go = Resolve-Go -Purpose "构建 Update_Applier"

# ---------- 图标资源处理（复用资源修复 Environment_Repair 同款图标） ----------
function Build-IconIfNeeded {
    if ($TargetOS -ne "windows" -or -not (Test-Path (Join-Path $ScriptDir "icon.ico"))) {
        return
    }

    if (Test-Path (Join-Path $ScriptDir "icon.syso")) {
        return
    }

    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) { throw "rsrc 图标编译失败" }
}

# ---------- 编译主流程 ----------
try {
    # 纯标准库、无 CGO 依赖
    $env:CGO_ENABLED = "0"
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    # Windows 下嵌入 .ico 到可执行文件（icon.syso 已随源码提供，缺失时重新生成）
    Push-Location $ScriptDir
    try {
        Build-IconIfNeeded
    }
    finally {
        Pop-Location
    }

    $binaryName = "Lunar_Update_Applier.exe"
    if ($TargetOS -ne "windows") { $binaryName = "Lunar_Update_Applier" }

    $outputPath = Join-Path $repoRoot $binaryName

    $ldflags = "-s -w"
    $buildArgs = @(
        "build",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )

    Push-Location $ScriptDir
    try {
        & $Go $buildArgs 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }
    }
    finally {
        Pop-Location
    }

    Write-Host "更新器构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] 更新器构建失败: $_" -ForegroundColor Red
    exit 1
}
