#!/bin/bash

echo "========================================"
echo "  月之华 - Webview 版本构建脚本"
echo "========================================"
echo ""

# 检查是否安装了 rsrc
if ! command -v rsrc &> /dev/null; then
    echo "[错误] 未找到 rsrc 工具"
    echo "请先安装 rsrc: go install github.com/akavel/rsrc@latest"
    exit 1
fi

# 检查是否存在图标文件
if [ ! -f "app.ico" ]; then
    echo "[警告] 未找到 app.ico 图标文件"
    echo "将继续构建但不包含图标"
else
    echo "[步骤 1/4] 生成资源文件..."
    rsrc -ico app.ico -o app.syso
    if [ $? -ne 0 ]; then
        echo "[错误] 资源文件生成失败"
        exit 1
    fi
    echo "[完成] 资源文件生成成功"
fi

echo ""
echo "[步骤 2/4] 启用 CGO..."
export CGO_ENABLED=1
echo "[完成] CGO 已启用"

echo ""
echo "[步骤 3/4] 检查系统依赖..."

# 检测操作系统
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    if ! command -v clang &> /dev/null; then
        echo "[错误] 未找到 clang 编译器"
        echo "请安装 Xcode 命令行工具: xcode-select --install"
        exit 1
    fi
    echo "[完成] macOS 系统检测完成"
    
    # 设置输出文件名
    OUTPUT_FILE="Lunar-Astral-Agents-webview-mac"
    
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux
    if ! dpkg -l | grep -q libgtk-3-dev; then
        echo "[错误] 未找到 GTK3 开发库"
        echo "请安装: sudo apt-get install libgtk-3-dev"
        exit 1
    fi
    echo "[完成] Linux 系统检测完成"
    
    # 设置输出文件名
    OUTPUT_FILE="Lunar-Astral-Agents-webview-linux"
else
    echo "[错误] 不支持的操作系统: $OSTYPE"
    exit 1
fi

echo ""
echo "[步骤 4/4] 构建 webview 版本..."
go build -tags webview -ldflags="-s -w" -o "$OUTPUT_FILE" main.go
if [ $? -ne 0 ]; then
    echo "[错误] 构建失败"
    exit 1
fi

echo ""
echo "========================================"
echo "  构建成功！"
echo "========================================"
echo ""
echo "生成的文件: $OUTPUT_FILE"
echo ""
echo "运行方式:"
echo "  1. 使用 webview 模式:"
echo "     ./$OUTPUT_FILE --use-webview=true"
echo ""
echo "  2. 自定义窗口大小:"
echo "     ./$OUTPUT_FILE --use-webview=true --webview-width=1920 --webview-height=1080"
echo ""
echo "  3. 查看所有参数:"
echo "     ./$OUTPUT_FILE --help"
echo ""
