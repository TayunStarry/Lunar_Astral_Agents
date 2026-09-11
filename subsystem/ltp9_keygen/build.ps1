# LTP9 Keygen - 编译脚本
# 将 LTP9 权限密钥生成器编译为独立可执行文件，输出到项目根目录
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

# 载入共享构建辅助（Go 工具链定位的唯一实现，见 subsystem/build_common.ps1）
$repoRoot = $ScriptRoot
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")

$Go = Resolve-Go -Purpose "构建 LTP9Keygen"
Write-Host "使用 Go: $Go"

# ---------- 编译主流程 ----------
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    if (-not (Test-Path (Join-Path $ScriptRoot "go.sum"))) {
        Write-Host "未找到 go.sum，执行 go mod tidy 解析依赖..."
        & $Go mod tidy
        if ($LASTEXITCODE -ne 0) { throw "go mod tidy 失败（网络受限可先配置 GOPROXY）" }
    }

    $binaryName = "LTP9Keygen.exe"
    if ($TargetOS -ne "windows") { $binaryName = "LTP9Keygen" }
    $outputPath = Join-Path (Split-Path -Parent $ScriptRoot) $binaryName   # subsystem 根目录

    $ldflags = "-s -w"
    if ($TargetOS -eq "windows") { $ldflags += " -H windowsgui" }
    $buildArgs = @(
        "build",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )

    Write-Host "构建 LTP9 密钥生成器 -> $outputPath"
    $buildOut = & $Go $buildArgs 2>&1
    $exitCode = $LASTEXITCODE
    $buildOut | ForEach-Object { Write-Host $_ }
    if ($exitCode -ne 0) { throw "Go build 失败（退出码 $exitCode）" }

    Write-Host "LTP9 密钥生成器构建成功: $outputPath" -ForegroundColor Green
    Write-Host "运行: $outputPath"
}
catch {
    Write-Host "`n[ERROR] LTP9 Keygen 构建失败: $_" -ForegroundColor Red
    exit 1
}