# Core Personality - 编译脚本
# 统一构建「核心人格」（月华 + 琉璃 合并工程，subsystem/core_personality）：
#   1. 图标编译（rsrc）
#   2. 关闭运行中的 Core_Personality.exe
#   3. QwenTTS / OmniVoiceTTS 引擎库按需构建（GGML → C++，产出运行时动态加载的 DLL）
#   4. 月华智能体 TypeScript 打包（tsc + rollup → agentSystem.js）
#   5. 端点自述文档生成（解析 server/variable.go 的统一路由注册表）
#   6. CGO 编译输出 ../Core_Personality.exe
#
# 注：C++ 引擎库与 TypeScript 打包均以「脚本所在目录」为基准、自身向上查找仓库根，
#     因此可安全地被 subsystem/build.ps1 或手工从任意目录调用。

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    # 强制重建两个 TTS 引擎库（即使对应 DLL 已存在）
    [switch]$WithTTS,

    # 跳过 TTS 引擎库构建（即使对应 DLL 不存在）
    [switch]$SkipTTS
)

$ErrorActionPreference = "Stop"
$ScriptDir = $PSScriptRoot

# 载入共享构建辅助（Go 工具链定位：PATH -> 常见目录 -> %USERPROFILE%\sdk\go*）
$repoRoot = $ScriptDir
while ($repoRoot -and -not (Test-Path (Join-Path $repoRoot "subsystem\build_common.ps1"))) {
    $repoRoot = Split-Path -Parent $repoRoot
}
if (-not $repoRoot) { throw "未找到仓库根目录（subsystem\build_common.ps1）" }
. (Join-Path $repoRoot "subsystem\build_common.ps1")
$Go = Resolve-Go -Purpose "构建 Core_Personality"

# ---------- 图标资源处理 ----------
function Build-IconIfNeeded {
    if ($TargetOS -ne "windows" -or -not (Test-Path (Join-Path $ScriptDir "icon.ico"))) {
        return
    }
    if (Test-Path (Join-Path $ScriptDir "icon.syso")) {
        return
    }

    Push-Location $ScriptDir
    try {
        if (-not (Get-Command rsrc -ErrorAction SilentlyContinue)) {
            # 图标是装饰性资源：rsrc 缺失时告警跳过，不阻断构建
            Write-Host "未找到 rsrc 工具，跳过图标编译（可执行 go install github.com/akavel/rsrc@latest 安装）" -ForegroundColor Yellow
            return
        }
        & rsrc -ico icon.ico -o icon.syso
        if ($LASTEXITCODE -ne 0) { throw "rsrc 图标编译失败" }
    }
    finally {
        Pop-Location
    }
}

# ---------- 辅助函数：执行命令并保留退出码 ----------
function Invoke-NativeCommand {
    param(
        [scriptblock]$ScriptBlock
    )

    # 临时将 ErrorActionPreference 改为 Continue，避免 stderr 输出被当作错误
    $originalEAP = $ErrorActionPreference
    $ErrorActionPreference = "Continue"

    try {
        & $ScriptBlock 2>&1 | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) {
                Write-Host $_.Exception.Message -ForegroundColor DarkYellow
            } else {
                Write-Host $_
            }
        }
        return $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $originalEAP
    }
}

# ---------- 关闭已启动的核心人格服务 ----------
# 运行中的进程会占用输出可执行文件，且会锁住 TTS 引擎 DLL，必须先关闭
function Stop-RunningCore {
    $processes = Get-Process -Name "Core_Personality" -ErrorAction SilentlyContinue
    if (-not $processes) {
        Write-Host "未检测到运行中的核心人格服务" -ForegroundColor DarkGray
        return
    }

    foreach ($process in $processes) {
        try {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
            Write-Host "已关闭核心人格服务 (PID $($process.Id))" -ForegroundColor Green
        }
        catch {
            Write-Host "关闭核心人格服务失败 (PID $($process.Id)): $_" -ForegroundColor Yellow
        }
    }

    # 等待进程退出并释放文件句柄
    Start-Sleep -Milliseconds 500
}

# ---------- 清除目录内残留的可执行文件 ----------
function Remove-StaleExe {
    foreach ($name in @("Core_Personality.exe", "Core_Personality")) {
        $staleExe = Join-Path $ScriptDir $name
        if (Test-Path $staleExe) {
            Remove-Item $staleExe -Force
            Write-Host "已清除目录内残留的 $name" -ForegroundColor Green
        }
    }
}

# ---------- TTS 引擎库按需构建 ----------
# 策略（与两个原工程一致）：对应 DLL 不存在则默认构建，已存在则默认跳过；
# -WithTTS 强制重建，-SkipTTS 强制跳过。
# 可执行文件运行时经 LoadLibraryExW + GetProcAddress 从模型目录动态加载这两个 DLL。
function Invoke-TtsLibBuildIfNeeded {
    param(
        [string]$Name,        # 引擎名（日志用）
        [string]$DllPath,     # 产出 DLL 路径（存在即视为已构建）
        [string]$BuildScript  # 引擎构建脚本
    )

    if ($SkipTTS) {
        Write-Host "已跳过 $Name 引擎库构建 (-SkipTTS)" -ForegroundColor DarkGray
        return
    }

    if ((Test-Path $DllPath) -and -not $WithTTS) {
        Write-Host "$(Split-Path $DllPath -Leaf) 已存在，跳过 $Name 引擎库构建（-WithTTS 可强制重建）" -ForegroundColor DarkGray
        return
    }

    if (-not (Test-Path $BuildScript)) {
        throw "未找到 $Name 构建脚本: $BuildScript"
    }

    Write-Host "开始构建 $Name 引擎库（GGML → C++）..." -ForegroundColor Cyan
    # 用独立 powershell 进程执行：子脚本以 exit 返回退出码，避免污染当前脚本作用域
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $BuildScript -TargetOS $TargetOS -TargetArch $TargetArch
    if ($LASTEXITCODE -ne 0) {
        throw "$Name 引擎库构建失败 (exit code: $LASTEXITCODE)"
    }
}

# ---------- 月华智能体 TypeScript 打包 ----------
# tsc 依据 tsconfig 编译 personality/lunar/TypeScript，rollup 打包为
# personality/lunar/hierarchy/assets/agentSystem.js（由 Go 侧 go:embed 嵌入）。
function Invoke-AgentBundleBuild {
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        throw "未找到 npm，无法打包智能体脚本（请安装 Node.js）"
    }

    Push-Location $ScriptDir
    try {
        if (-not (Test-Path (Join-Path $ScriptDir "node_modules"))) {
            Write-Host "node_modules 缺失，执行 npm install..." -ForegroundColor Yellow
            $installExit = Invoke-NativeCommand { npm install --no-audit --no-fund }
            if ($installExit -ne 0) { throw "npm install 执行失败" }
        }

        Write-Host "打包月华智能体脚本（npm run server.side）..." -ForegroundColor Cyan
        $exitCode = Invoke-NativeCommand { npm run server.side }
        if ($exitCode -ne 0) { throw "npm run server.side 执行失败" }
    }
    finally {
        Pop-Location
    }

    $bundle = Join-Path $ScriptDir "personality\lunar\hierarchy\assets\agentSystem.js"
    if (-not (Test-Path $bundle)) {
        throw "智能体脚本未产出: $bundle"
    }
    $bundleSize = [math]::Round((Get-Item $bundle).Length / 1KB, 1)
    Write-Host "  [OK] agentSystem.js ($bundleSize KB)" -ForegroundColor Green
}

# ---------- 编译时生成端点自述文档 ----------
# 调用共享生成器解析 server/variable.go 中的 SystemEndpoints 统一路由注册表，
# 生成 server/endpoint_docs.gen.json（由 server 包经 go:embed 嵌入，经 /api-docs 提供）。
# 生成器运行在宿主平台，必须在设置 GOOS/GOARCH 之前执行。
function Invoke-EndpointDocsGenerate {
    Write-Host "生成端点自述文档（编译时）..." -ForegroundColor DarkGray
    $genDocArgs = @(
        "run", "./cmd/generator",
        "-service", "CorePersonality",
        "-display", "核心人格 (Core Personality)",
        "-src", (Join-Path $ScriptDir "server\variable.go"),
        "-out", (Join-Path $ScriptDir "server\endpoint_docs.gen.json"),
        "-docs-path", "/api-docs",
        "-extra", "GET|/|月华消息终端静态前端（内嵌 hierarchy 资源）",
        "-extra", "GET|/desktop/|琉璃桌面静态前端（内嵌 web 资源）",
        "-extra", "GET|/ws|WebSocket 消息通道（结构化消息投递给全部连接；引擎总线原始帧仅投递给显式订阅 ?bus=1 的连接）"
    )
    Push-Location (Join-Path $repoRoot "subsystem\endpoint_docs")
    try {
        $genExitCode = Invoke-NativeCommand { & $Go @genDocArgs }
        if ($genExitCode -ne 0) { throw "端点自述文档生成失败" }
    }
    finally {
        Pop-Location
    }
}

# ---------- 编译主流程 ----------
try {
    Write-Host ""
    Write-Host "  Core Personality 构建 (目标: $TargetOS / $TargetArch)" -ForegroundColor Cyan
    Write-Host ""

    # 编译图标
    Build-IconIfNeeded

    # 关闭已启动的核心人格服务
    Stop-RunningCore

    # 清除目录内可能残留的可执行文件
    Remove-StaleExe

    # 按需构建两个 TTS 引擎库（须在关闭服务之后：运行中的进程会锁住 DLL）
    Invoke-TtsLibBuildIfNeeded -Name "QwenTTS" `
        -DllPath (Join-Path $repoRoot "local_data\models\Qwen3-TTS\qwen3tts.dll") `
        -BuildScript (Join-Path $ScriptDir "engine\qwen_tts\build.ps1")
    Invoke-TtsLibBuildIfNeeded -Name "OmniVoiceTTS" `
        -DllPath (Join-Path $repoRoot "local_data\models\OmniVoice\omnivoice.dll") `
        -BuildScript (Join-Path $ScriptDir "engine\omnivoice_tts\build.ps1")

    # 打包月华智能体脚本（须在 go build 之前：agentSystem.js 经 go:embed 嵌入）
    Invoke-AgentBundleBuild

    # 生成端点自述文档（须在设置 GOOS/GOARCH 之前）
    Invoke-EndpointDocsGenerate

    # 启用 CGO（sqlite3 / onnxruntime 等依赖 C 运行时）
    $env:CGO_ENABLED = "1"
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    if ($TargetOS -eq "windows" -and -not (Get-Command gcc -ErrorAction SilentlyContinue)) {
        throw "未找到 GCC，CGO 编译需要 MinGW-w64 或 TDM-GCC"
    }

    Push-Location $ScriptDir
    try {
        $binaryName = "Core_Personality.exe"
        if ($TargetOS -ne "windows") { $binaryName = "Core_Personality" }
        # 输出到仓库根目录：可执行文件按自身所在目录解析 local_data / 图标 / 模型目录，
        # 本工程位于 subsystem/core_personality，故需上溯两级。
        $outputPath = "..\..\$binaryName"

        # Windows 桌面应用：隐藏控制台窗口（启动失败经失败音效与日志提示）
        $ldflags = "-s -w"
        # if ($TargetOS -eq "windows") { $ldflags += " -H windowsgui" }

        $buildArgs = @(
            "build",
            "-tags", "webview",
            "-ldflags=$ldflags",
            "-trimpath",
            "-o", $outputPath
        )
        $exitCode = Invoke-NativeCommand { & $Go $buildArgs }
        if ($exitCode -ne 0) { throw "Go build 失败" }
    }
    finally {
        Pop-Location
    }

    Write-Host ""
    Write-Host "✓ Core Personality 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] Core Personality 构建失败: $_" -ForegroundColor Red
    exit 1
}