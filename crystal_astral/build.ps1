# Crystal Astral - 编译脚本
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

# ---------- 编译主流程 ----------
try {
    # 编译图标
    Build-IconIfNeeded

    # 关闭已启动的琉璃服务
    Stop-RunningCrystal

    # 清除目录内可能残留的 Crystal_Astral.exe
    Remove-StaleExe

    # 启用CGO：ASR 能力（qwen_asr）依赖 cgo 编译 C 源码并链接 OpenBLAS
    $env:CGO_ENABLED = "1"
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch

    # CGO 编译 C 源码需要 GCC（MinGW-w64）
    if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
        throw "未找到 GCC，ASR 能力需要 CGO 编译器（请安装 MinGW-w64 或 TDM-GCC）"
    }

    # 构建可执行文件
    $binaryName = "Crystal_Astral.exe"
    if ($TargetOS -ne "windows") { $binaryName = "Crystal_Astral" }
    $outputPath = "..\$binaryName"

    # 仅 Windows 平台使用 windowsgui 头部（隐藏控制台窗口）
    $ldflags = "-s -w"
    #if ($TargetOS -eq "windows") { $ldflags += " -H windowsgui" }
    
    $buildArgs = @(
        "build",
        "-tags", "webview",
        "-ldflags=$ldflags",
        "-trimpath",
        "-o", $outputPath
    )
    & go $buildArgs 2>&1 | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Go build 失败" }

    Write-Host "✓ Crystal Astral 构建成功: $outputPath" -ForegroundColor Green
}
catch {
    Write-Host "`n[ERROR] Crystal Astral 构建失败: $_" -ForegroundColor Red
    exit 1
}
