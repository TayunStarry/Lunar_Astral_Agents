# proxy - 编译脚本
# 由根目录脚本统一调用，仅处理项目编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"

# ---------- 编译主流程 ----------
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    $ldflags = "-s -w -H windowsgui"

    $binaryName = "proxy_server.exe"
    if ($TargetOS -ne "windows") { $binaryName = "proxy_server" }
    $outputPath = "..\..\$binaryName"

    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath,
        "./cmd/"
    )
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }

    Write-Host "✓ proxy_server 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] proxy_server 构建失败: $_" -ForegroundColor Red
    exit 1
}
