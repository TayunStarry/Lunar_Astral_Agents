# Crystal Astral - 编译脚本
# 由根目录脚本统一调用，处理图标编译、OmniVoice 引擎库按需构建和项目编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    # 强制重建 OmniVoice 引擎库（即使 omnivoice.dll 已存在）
    [switch]$WithTTS,

    # 跳过 OmniVoice 引擎库构建（即使 omnivoice.dll 不存在）
    [switch]$SkipTTS
)

$ErrorActionPreference = "Stop"

# 载入共享构建辅助（Go 工具链定位：PATH -> 常见目录 -> %USERPROFILE%\sdk\go*）
$repoRoot = $PSScriptRoot
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")
$Go = Resolve-Go -Purpose "构建 Crystal_Astral"

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

# ---------- 关闭已启动的琉璃服务 ----------
# 运行中的琉璃会占用输出可执行文件，不先关闭会导致覆盖写入失败
function Stop-RunningCrystal {
    $processes = Get-Process -Name "Crystal_Astral" -ErrorAction SilentlyContinue
    if (-not $processes) {
        Write-Host "未检测到运行中的琉璃服务" -ForegroundColor DarkGray
        return
    }

    foreach ($process in $processes) {
        try {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
            Write-Host "已关闭琉璃服务 (PID $($process.Id))" -ForegroundColor Green
        }
        catch {
            Write-Host "关闭琉璃服务失败 (PID $($process.Id)): $_" -ForegroundColor Yellow
        }
    }

    # 等待进程退出并释放文件句柄
    Start-Sleep -Milliseconds 500
}

# ---------- 清除目录内残留的可执行文件 ----------
# 历史构建/手工复制可能在项目目录内留下 Crystal_Astral.exe，编译前统一清除
function Remove-StaleExe {
    $staleExe = Join-Path $PSScriptRoot "Crystal_Astral.exe"
    if (Test-Path $staleExe) {
        Remove-Item $staleExe -Force
        Write-Host "已清除目录内残留的 Crystal_Astral.exe: $staleExe" -ForegroundColor Green
    }
}

# ---------- OmniVoice 引擎库按需构建 ----------
# 策略：omnivoice.dll（local_data\models\OmniVoice）不存在 → 默认构建；
# 已存在 → 默认跳过。-WithTTS 强制重建，-SkipTTS 强制跳过。
# engine\OmniVoiceTTS\build.ps1 会产出 omnivoice.dll 并连同依赖 DLL
# （vulkan-1/libgomp-1/libwinpthread-1）一起输出到模型目录。
function Invoke-TtsBuildIfNeeded {
    if ($SkipTTS) {
        Write-Host "已跳过 OmniVoice 引擎库构建 (-SkipTTS)" -ForegroundColor DarkGray
        return
    }

    $ttsDll = Join-Path $repoRoot "local_data\models\OmniVoice\omnivoice.dll"
    if ((Test-Path $ttsDll) -and -not $WithTTS) {
        Write-Host "omnivoice.dll 已存在，跳过引擎库构建（-WithTTS 可强制重建）" -ForegroundColor DarkGray
        return
    }

    $ttsBuildScript = Join-Path $PSScriptRoot "engine\OmniVoiceTTS\build.ps1"
    if (-not (Test-Path $ttsBuildScript)) {
        throw "未找到 OmniVoice 构建脚本: $ttsBuildScript"
    }

    Write-Host "开始构建 OmniVoice 引擎库（GGML → C++）..." -ForegroundColor Cyan
    # 用独立 powershell 进程执行：子脚本以 exit 返回退出码，避免污染当前脚本作用域
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $ttsBuildScript -TargetOS $TargetOS -TargetArch $TargetArch
    if ($LASTEXITCODE -ne 0) {
        throw "OmniVoice 引擎库构建失败 (exit code: $LASTEXITCODE)"
    }
}

# ---------- 编译主流程 ----------
try {
    # 编译图标
    Build-IconIfNeeded

    # 关闭已启动的琉璃服务
    Stop-RunningCrystal

    # 清除目录内可能残留的 Crystal_Astral.exe
    Remove-StaleExe

    # 按需构建 OmniVoice 引擎库（须在关闭琉璃服务之后：运行中的进程会锁住 omnivoice.dll）
    Invoke-TtsBuildIfNeeded

    # ---------- 编译时生成端点自述文档 ----------
    # 调用共享生成器解析 variable.go 中的 SystemEndpoints 注册表，生成 endpoint_docs.gen.json
    # （由 docs_endpoint.go 经 go:embed 嵌入，经 /api-docs 端点对外提供）。
    # 生成器运行在宿主平台，必须在下方设置 GOOS/GOARCH 之前执行
    Write-Host "生成端点自述文档（编译时）..." -ForegroundColor DarkGray
    $genDocArgs = @(
        "run", "./cmd/generator",
        "-service", "CrystalAstral",
        "-display", "琉璃 (Crystal Astral)",
        "-src", (Join-Path $PSScriptRoot "variable.go"),
        "-out", (Join-Path $PSScriptRoot "endpoint_docs.gen.json"),
        "-docs-path", "/api-docs",
        "-extra", "WS|/ws|工作室 WebSocket 消息通道（无差别广播，客户端自行过滤）"
    )
    Push-Location (Join-Path $repoRoot "subsystem\endpoint_docs")
    try {
        & $Go @genDocArgs 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "端点自述文档生成失败" }
    }
    finally {
        Pop-Location
    }

    # 启用CGO：ASR 能力（qwen_asr）依赖 cgo 编译 C 源码并链接 OpenBLAS
    $env:CGO_ENABLED = "1"
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    # CGO 编译 C 源码需要 GCC（MinGW-w64）
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
        throw "未找到 GCC，ASR 能力需要 CGO 编译器（请安装 MinGW-w64 或 TDM-GCC）"
    }

    # 构建可执行文件（以脚本所在目录为基准，不依赖调用方 cwd）
    Push-Location $PSScriptRoot
    try {
        $binaryName = "Crystal_Astral.exe"
        if ($TargetOS -ne "windows") { $binaryName = "Crystal_Astral" }
        $outputPath = "..\$binaryName"

        # 仅 Windows 平台使用 windowsgui 头部（隐藏控制台窗口）
        $ldflags = "-s -w"
        if ($TargetOS -eq "windows") { $ldflags += " -H windowsgui" }

        $buildArgs = @(
            "build",
            "-tags", "webview",
            "-ldflags=$ldflags",
            "-trimpath",
            "-o", $outputPath
        )
        & $Go $buildArgs 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }
    }
    finally {
        Pop-Location
    }

    Write-Host "✓ Crystal Astral 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] Crystal Astral 构建失败: $_" -ForegroundColor Red
    exit 1
}
