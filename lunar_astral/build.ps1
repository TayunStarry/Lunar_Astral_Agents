# Luna Astral - 编译脚本
# 由根目录脚本统一调用，处理图标编译、QwenTTS 引擎库按需构建和项目编译

param(
    [ValidateSet("windows", "linux", "darwin")]
    [string]$TargetOS = "windows",
    [ValidateSet("amd64", "arm64")]
    [string]$TargetArch = "amd64",

    # 强制重建 QwenTTS 引擎库（即使 qwen3tts.dll 已存在）
    [switch]$WithTTS,

    # 跳过 QwenTTS 引擎库构建（即使 qwen3tts.dll 不存在）
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
$Go = Resolve-Go -Purpose "构建 Lunar_Astral"

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

# ---------- 关闭已启动的月华服务 ----------
# 运行中的月华会占用输出可执行文件，不先关闭会导致覆盖写入失败
function Stop-RunningLunar {
    $processes = Get-Process -Name "Lunar_Astral" -ErrorAction SilentlyContinue
    if (-not $processes) {
        Write-Host "未检测到运行中的月华服务" -ForegroundColor DarkGray
        return
    }

    foreach ($process in $processes) {
        try {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
            Write-Host "已关闭月华服务 (PID $($process.Id))" -ForegroundColor Green
        }
        catch {
            Write-Host "关闭月华服务失败 (PID $($process.Id)): $_" -ForegroundColor Yellow
        }
    }

    # 等待进程退出并释放文件句柄
    Start-Sleep -Milliseconds 500
}

# ---------- QwenTTS 引擎库按需构建 ----------
function Invoke-TtsBuildIfNeeded {
    if ($SkipTTS) {
        Write-Host "已跳过 QwenTTS 引擎库构建 (-SkipTTS)" -ForegroundColor DarkGray
        return
    }

    $ttsDll = Join-Path $repoRoot "local_data\models\Qwen3-TTS\qwen3tts.dll"
    if ((Test-Path $ttsDll) -and -not $WithTTS) {
        Write-Host "qwen3tts.dll 已存在，跳过引擎库构建（-WithTTS 可强制重建）" -ForegroundColor DarkGray
        return
    }

    $ttsBuildScript = Join-Path $PSScriptRoot "engine\QwenTTS\build.ps1"
    if (-not (Test-Path $ttsBuildScript)) {
        throw "未找到 QwenTTS 构建脚本: $ttsBuildScript"
    }

    Write-Host "开始构建 QwenTTS 引擎库（GGML → C++）..." -ForegroundColor Cyan
    # 用独立 powershell 进程执行：子脚本以 exit 返回退出码，避免污染当前脚本作用域
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $ttsBuildScript -TargetOS $TargetOS -TargetArch $TargetArch
    if ($LASTEXITCODE -ne 0) {
        throw "QwenTTS 引擎库构建失败 (exit code: $LASTEXITCODE)"
    }
}

# ---------- 清除目录内残留的可执行文件 ----------
# 历史构建/手工复制可能在项目目录内留下 Lunar_Astral.exe，编译前统一清除
function Remove-StaleExe {
    $staleExe = Join-Path $PSScriptRoot "Lunar_Astral.exe"
    if (Test-Path $staleExe) {
        Remove-Item $staleExe -Force
        Write-Host "已清除目录内残留的 Lunar_Astral.exe: $staleExe" -ForegroundColor Green
    }
}

# ---------- 编译主流程 ----------
try {
    # 编译图标
    Build-IconIfNeeded

    # 关闭已启动的月华服务
    Stop-RunningLunar

    # 清除目录内可能残留的 Lunar_Astral.exe
    Remove-StaleExe

    # 按需构建 QwenTTS 引擎库（须在关闭月华服务之后：运行中的进程会锁住 qwen3tts.dll）
    Invoke-TtsBuildIfNeeded

    # ---------- 编译时生成端点自述文档 ----------
    # 调用共享生成器解析 server/variable.go 中的 SystemEndpoints 注册表，生成 server/endpoint_docs.gen.json
    # （由 server 包经 go:embed 嵌入，经 /api-docs 端点对外提供）。
    # 生成器运行在宿主平台，必须在下方设置 GOOS/GOARCH 之前执行
    Write-Host "生成端点自述文档（编译时）..." -ForegroundColor DarkGray
    $genDocArgs = @(
        "run", "./cmd/generator",
        "-service", "LunarAstral",
        "-display", "月华 (Lunar Astral)",
        "-src", (Join-Path $PSScriptRoot "server\variable.go"),
        "-out", (Join-Path $PSScriptRoot "server\endpoint_docs.gen.json"),
        "-docs-path", "/api-docs",
        "-extra", "WS|/ws|WebSocket 消息通道（服务端向已连接客户端广播，客户端自行过滤）"
    )
    Push-Location (Join-Path $repoRoot "subsystem\endpoint_docs")
    try {
        $genExitCode = Invoke-NativeCommand { & $Go @genDocArgs }
        if ($genExitCode -ne 0) { throw "端点自述文档生成失败" }
    }
    finally {
        Pop-Location
    }

    # 启用CGO
    $env:CGO_ENABLED = 1
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_CFLAGS = "-w"
    $env:CGO_LDFLAGS = "-static-libgcc -static-libstdc++ -Wl,-Bstatic,-lwinpthread,-Bdynamic"

    # npm/Go 构建均以脚本所在目录为基准（不依赖调用方的当前目录）
    Push-Location $PSScriptRoot
    try {
        # 编译服务端脚本
        $exitCode = Invoke-NativeCommand { npm run server.side }
        if ($exitCode -ne 0) { throw "npm server.side 执行失败" }

        # 构建可执行文件
        $binaryName = "Lunar_Astral.exe"
        if ($TargetOS -ne "windows") { $binaryName = "Lunar_Astral" }
        $outputPath = "..\$binaryName"

        $ldflags = "-s -w -extldflags=-Wl,-Bstatic,-lstdc++,-lgcc,-lgcc_eh,-lwinpthread,-Bdynamic"
        $buildArgs = @(
            "build",
            "-tags", "webview",
            "-ldflags", $ldflags,
            "-o", $outputPath
        )
        $exitCode = Invoke-NativeCommand { & $Go $buildArgs }
        if ($exitCode -ne 0) { throw "Go build 失败" }
    }
    finally {
        Pop-Location
    }

    Write-Host "✓ Luna Astral 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] Luna Astral 构建失败: $_" -ForegroundColor Red
    exit 1
}
