# ============================================================
# sd_lunar 构建脚本
# 1. 使用 CMake 编译 native/ 目录下的 C/C++ 静态库
# 2. 使用 Go CGO 编译 Go 模块并链接静态库
# ============================================================

param(
    [switch]$Clean,
    [switch]$Release,
    [switch]$CPUOnly,
    [string]$OutputName = "sd_lunar.exe"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  sd_lunar 构建脚本 - 月华出品 ^_^" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""

$NativeDir = Join-Path $ScriptDir "native"
$BuildDir = Join-Path $NativeDir "build"

if ($Clean) {
    Write-Host "[清理] 正在清理构建产物..." -ForegroundColor Yellow
    if (Test-Path $BuildDir) {
        Remove-Item -Recurse -Force $BuildDir
        Write-Host "[清理] 已删除 CMake 构建目录" -ForegroundColor Green
    }
    Remove-Item -Force -ErrorAction SilentlyContinue $OutputName
    Remove-Item -Force -ErrorAction SilentlyContinue "*.exp"
    Remove-Item -Force -ErrorAction SilentlyContinue "*.lib"
    Write-Host "[清理] 构建产物清理完成" -ForegroundColor Green
    Write-Host ""
}

Write-Host "[步骤 1/2] 编译 C/C++ 原生静态库..." -ForegroundColor Cyan
Write-Host ""

if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null
}

Push-Location $BuildDir

if ($Release) {
    $cmakeBuildType = "Release"
} else {
    $cmakeBuildType = "Release"
}

try {
    $cmakeArgs = @(
        "..",
        "-DCMAKE_BUILD_TYPE=$cmakeBuildType",
        "-G", "Visual Studio 17 2022"
    )

    if ($CPUOnly) {
        $cmakeArgs += "-DSD_VULKAN=OFF"
        Write-Host "[CMake] Vulkan 已禁用 (CPU Only 模式)" -ForegroundColor Yellow
    } else {
        Write-Host "[CMake] Vulkan GPU 加速已启用" -ForegroundColor Green
    }

    Write-Host "[CMake] 配置中: cmake $($cmakeArgs -join ' ')" -ForegroundColor Gray
    $cmakeConfig = Start-Process -FilePath "cmake" -ArgumentList $cmakeArgs -NoNewWindow -Wait -PassThru
    if ($cmakeConfig.ExitCode -ne 0) {
        Write-Host "[错误] CMake 配置失败 (退出码: $($cmakeConfig.ExitCode))" -ForegroundColor Red
        Write-Host "[提示] 请检查是否安装了 CMake 和 Visual Studio 2022" -ForegroundColor Yellow
        Pop-Location
        exit 1
    }

    Write-Host "[CMake] 编译中..." -ForegroundColor Gray
    $cmakeBuild = Start-Process -FilePath "cmake" -ArgumentList @("--build", ".", "--config", $cmakeBuildType) -NoNewWindow -Wait -PassThru
    if ($cmakeBuild.ExitCode -ne 0) {
        Write-Host "[错误] CMake 编译失败 (退出码: $($cmakeBuild.ExitCode))" -ForegroundColor Red
        Pop-Location
        exit 1
    }

    Write-Host "[CMake] 原生静态库编译成功!" -ForegroundColor Green
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "[步骤 2/2] 编译 Go CGO 模块..." -ForegroundColor Cyan
Write-Host ""

$env:CGO_ENABLED = "1"

Push-Location $ScriptDir

try {
    if ($Release) {
        $ldFlags = "-s -w"
    } else {
        $ldFlags = ""
    }

    $goArgs = @("build")
    if ($ldFlags) {
        $goArgs += "-ldflags=$ldFlags"
    }
    $goArgs += @("-v", "-o", $OutputName, ".")

    Write-Host "[Go] go $($goArgs -join ' ')" -ForegroundColor Gray

    $goBuild = Start-Process -FilePath "go" -ArgumentList $goArgs -NoNewWindow -Wait -PassThru

    if ($goBuild.ExitCode -ne 0) {
        Write-Host "[错误] Go 编译失败 (退出码: $($goBuild.ExitCode))" -ForegroundColor Red
        exit 1
    }

    Write-Host ""
    Write-Host "============================================" -ForegroundColor Green
    Write-Host "  构建成功! 输出: $OutputName" -ForegroundColor Green
    Write-Host "============================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "运行方式:" -ForegroundColor Cyan
    Write-Host "  .\$OutputName" -ForegroundColor Gray
    Write-Host ""
    Write-Host "API接口:" -ForegroundColor Cyan
    Write-Host "  POST /api/v1/txt2img  - 文生图" -ForegroundColor Gray
    Write-Host "  POST /api/v1/img2img  - 图生图" -ForegroundColor Gray
    Write-Host "  GET  /api/v1/status   - 状态查询" -ForegroundColor Gray
    Write-Host "  GET  /api/v1/ping     - 健康检查" -ForegroundColor Gray

} finally {
    Pop-Location
}