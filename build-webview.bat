@echo off
chcp 65001 >nul
echo ========================================
echo   月之华 - Webview 版本构建脚本
echo ========================================
echo.

REM 检查是否安装了 rsrc
where rsrc >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未找到 rsrc 工具
    echo 请先安装 rsrc: go install github.com/akavel/rsrc@latest
    pause
    exit /b 1
)

REM 检查是否存在图标文件
if not exist "app.ico" (
    echo [警告] 未找到 app.ico 图标文件
    echo 将继续构建但不包含图标
) else (
    echo [步骤 1/4] 生成资源文件...
    rsrc -ico app.ico -o app.syso
    if %errorlevel% neq 0 (
        echo [错误] 资源文件生成失败
        pause
        exit /b 1
    )
    echo [完成] 资源文件生成成功
)

echo.
echo [步骤 2/4] 启用 CGO...
set CGO_ENABLED=1
echo [完成] CGO 已启用

echo.
echo [步骤 3/4] 检查 GCC 编译器...
where gcc >nul 2>&1
if %errorlevel% neq 0 (
    echo [错误] 未找到 GCC 编译器
    echo 请安装 MinGW-w64 或 TDM-GCC
    echo 下载地址: https://www.mingw-w64.org/
    pause
    exit /b 1
)
echo [完成] GCC 编译器已找到

echo.
echo [步骤 4/4] 构建 webview 版本...
go build -tags webview -ldflags="-s -w" -o Lunar-Astral-Agents-webview.exe main.go
if %errorlevel% neq 0 (
    echo [错误] 构建失败
    pause
    exit /b 1
)

echo.
echo ========================================
echo   构建成功！
echo ========================================
echo.
echo 生成的文件: Lunar-Astral-Agents-webview.exe
echo.
echo 运行方式:
echo   1. 使用 webview 模式:
echo      Lunar-Astral-Agents-webview.exe --use-webview=true
echo.
echo   2. 自定义窗口大小:
echo      Lunar-Astral-Agents-webview.exe --use-webview=true --webview-width=1920 --webview-height=1080
echo.
echo   3. 查看所有参数:
echo      Lunar-Astral-Agents-webview.exe --help
echo.
pause
