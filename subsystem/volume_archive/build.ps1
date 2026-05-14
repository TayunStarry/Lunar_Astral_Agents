# volume_archive - 编译脚本
# 由根目录脚本统一调用，仅处理项目编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"

try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 0

    $ldflags = "-s -w"

    $outputName = "volume_archive.exe"
    if ($TargetOS -ne "windows") {
        $outputName = "volume_archive"
    }
    $outputPath = "..\..\$outputName"

    $buildArgs = @(
        "build",
        "-ldflags", $ldflags,
        "-o", $outputPath
    )
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }

    Write-Host "✓ volume_archive 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] volume_archive 构建失败: $_" -ForegroundColor Red
    exit 1
}
