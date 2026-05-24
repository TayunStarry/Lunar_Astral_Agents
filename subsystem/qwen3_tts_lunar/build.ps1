# bridge_adapter - 编译脚本
# 由根目录脚本统一调用，仅处理图标编译和项目编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64"
)

$ErrorActionPreference = "Stop"

# ---------- 图标资源处理 ----------
function Build-IconIfNeeded {
    if ($TargetOS -ne "windows" -or -not (Test-Path "icon.ico")) {
        return
    }

    if (Test-Path "icon.syso") {
        return
    }

    & rsrc -ico icon.ico -o icon.syso
    if ($LASTEXITCODE -ne 0) { throw "rsrc 图标编译失败" }
}

# ---------- 编译主流程 ----------
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = 1

    Build-IconIfNeeded

    $ldflags = "-s -w"
    # $ldflags = "-s -w -H windowsgui"

    $binaryName = "Qwen3_TTS_Lunar.exe"
    if ($TargetOS -ne "windows") { $binaryName = "Qwen3_TTS_Lunar" }
    $outputPath = "..\..\$binaryName"

    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }

    Write-Host "✓ Qwen3_TTS_Lunar 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] Qwen3_TTS_Lunar 构建失败: $_" -ForegroundColor Red
    exit 1
}
