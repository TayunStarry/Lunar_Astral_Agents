# LTP3 Keygen - 编译脚本
# 将 LTP3 权限密钥生成器编译为独立可执行文件，输出到项目根目录
#
# 用法: .\build.ps1 [-TargetOS windows|linux|darwin] [-TargetArch amd64|arm64]

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"
$ScriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

# ---------- 定位 Go 工具链（PATH 优先，其次常见安装目录） ----------
function Find-Go {
    $cmd = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    foreach ($c in @(
        "C:\Program Files\Go\bin\go.exe",
        "C:\Go\bin\go.exe",
        (Join-Path $env:USERPROFILE "go\bin\go.exe")
    )) {
        if ($c -and (Test-Path $c)) { return $c }
    }
    if ($env:USERPROFILE) {
        Get-ChildItem (Join-Path $env:USERPROFILE "sdk") -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -like "go*" } |
            ForEach-Object { $p = Join-Path $_.FullName "bin\go.exe"; if (Test-Path $p) { return $p } }
    }
    return $null
}

$Go = Find-Go
if (-not $Go) {
    throw "未找到 Go 工具链，请安装 Go 或将其加入 PATH"
}
Write-Host "使用 Go: $Go"

# ---------- 编译主流程 ----------
try {
    # 目标平台与架构（webview 工具，默认 Windows/amd64）
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    # 模块依赖解析：go.sum 缺失时先整理依赖，避免 readonly 模式报错
    if (-not (Test-Path (Join-Path $ScriptRoot "go.sum"))) {
        Write-Host "未找到 go.sum，执行 go mod tidy 解析依赖..."
        & $Go mod tidy
        if ($LASTEXITCODE -ne 0) { throw "go mod tidy 失败（网络受限可先配置 GOPROXY）" }
    }

    $binaryName = "LTP3Keygen.exe"
    if ($TargetOS -ne "windows") { $binaryName = "LTP3Keygen" }
    $outputPath = Join-Path (Split-Path -Parent $ScriptRoot) $binaryName   # subsystem 根目录

    $ldflags = "-s -w"
    if ($TargetOS -eq "windows") { $ldflags += " -H windowsgui" }   # Windows 下隐藏控制台窗口
    $buildArgs = @(
        "build",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )

    Write-Host "构建 LTP3 密钥生成器 -> $outputPath"
    # 捕获输出后再判断退出码，避免 $ErrorActionPreference=Stop 被 go 的 stderr 首行触发而吞掉真实错误
    $buildOut = & $Go $buildArgs 2>&1
    $exitCode = $LASTEXITCODE
    $buildOut | ForEach-Object { Write-Host $_ }
    if ($exitCode -ne 0) { throw "Go build 失败（退出码 $exitCode）" }

    Write-Host "LTP3 密钥生成器构建成功: $outputPath" -ForegroundColor Green
    Write-Host "运行: $outputPath"
}
catch {
    Write-Host "`n[ERROR] LTP3 Keygen 构建失败: $_" -ForegroundColor Red
    exit 1
}
