# Webview CGO 编译指南

本文档详细说明如何使用 CGO 编译支持 webview 的月之华项目版本。

## 什么是 Webview？

Webview 是一个轻量级的跨平台库，允许在应用程序中嵌入 Web 浏览器组件。使用 webview 可以：

- 在独立窗口中打开 Web 界面
- 提供更好的用户体验
- 避免依赖系统默认浏览器
- 实现更紧密的应用集成

## 系统要求

### Windows

**必需软件：**
- MinGW-w64 或 TDM-GCC（提供 GCC 编译器）
- Go 1.21 或更高版本
- rsrc 工具（用于添加图标，可选）

**安装 MinGW-w64：**

1. 下载 MinGW-w64：https://www.mingw-w64.org/
2. 或使用 TDM-GCC：https://jmeubank.github.io/tdm-gcc/
3. 安装时选择 x86_64 架构
4. 将 MinGW 的 bin 目录添加到系统 PATH 环境变量

**验证安装：**
```bash
gcc --version
```

### macOS

**必需软件：**
- Xcode 命令行工具
- Go 1.21 或更高版本

**安装 Xcode 命令行工具：**
```bash
xcode-select --install
```

**验证安装：**
```bash
clang --version
```

### Linux

**必需软件：**
- GCC 编译器
- GTK3 开发库
- Go 1.21 或更高版本

**安装依赖（Ubuntu/Debian）：**
```bash
sudo apt-get update
sudo apt-get install build-essential libgtk-3-dev
```

**安装依赖（Fedora/RHEL）：**
```bash
sudo dnf install gcc gtk3-devel
```

**安装依赖（Arch Linux）：**
```bash
sudo pacman -S base-devel gtk3
```

**验证安装：**
```bash
gcc --version
pkg-config --modversion gtk+-3.0
```

## 编译步骤

### 方法一：使用构建脚本（推荐）

#### Windows

双击运行 `build-webview.bat` 脚本，或在命令行中执行：

```bash
build-webview.bat
```

#### macOS/Linux

给脚本添加执行权限并运行：

```bash
chmod +x build-webview.sh
./build-webview.sh
```

### 方法二：手动编译

#### 1. 安装 rsrc 工具（可选，用于添加图标）

```bash
go install github.com/akavel/rsrc@latest
```

#### 2. 生成资源文件（可选）

**Windows:**
```bash
rsrc -ico app.ico -o app.syso
```

**macOS/Linux:**
```bash
# macOS
rsrc -ico app.ico -o app.syso

# Linux
rsrc -ico app.ico -o app.syso
```

#### 3. 启用 CGO 并编译

**Windows:**
```bash
set CGO_ENABLED=1
go build -tags webview -ldflags="-s -w" -o Lunar-Astral-Agents-webview.exe main.go
```

**macOS:**
```bash
export CGO_ENABLED=1
go build -tags webview -ldflags="-s -w" -o Lunar-Astral-Agents-webview-mac main.go
```

**Linux:**
```bash
export CGO_ENABLED=1
go build -tags webview -ldflags="-s -w" -o Lunar-Astral-Agents-webview-linux main.go
```

## 编译参数说明

- `CGO_ENABLED=1`：启用 CGO 编译
- `-tags webview`：使用 webview 构建标签
- `-ldflags="-s -w"`：减小可执行文件大小
  - `-s`：去除符号表
  - `-w`：去除 DWARF 调试信息

## 运行 Webview 版本

### 基本运行

```bash
# Windows
Lunar-Astral-Agents-webview.exe --use-webview=true

# macOS
./Lunar-Astral-Agents-webview-mac --use-webview=true

# Linux
./Lunar-Astral-Agents-webview-linux --use-webview=true
```

### 自定义窗口参数

```bash
# 设置窗口大小
./Lunar-Astral-Agents-webview --use-webview=true --webview-width=1920 --webview-height=1080

# 设置窗口标题
./Lunar-Astral-Agents-webview --use-webview=true --webview-title="我的知识库"

# 固定窗口大小（不可调整）
./Lunar-Astral-Agents-webview --use-webview=true --webview-resizable=false

# 启用调试模式（显示开发者工具）
./Lunar-Astral-Agents-webview --use-webview=true --webview-debug=true
```

### 组合使用

```bash
./Lunar-Astral-Agents-webview \
  --use-webview=true \
  --webview-width=1920 \
  --webview-height=1080 \
  --webview-title="月之华 - 知识库浏览器" \
  --webview-resizable=true \
  --webview-debug=false \
  --basic-port=36789
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--use-webview` | false | 是否使用 webview 内嵌浏览器 |
| `--webview-title` | "月之华 - 知识库浏览器" | webview 窗口标题 |
| `--webview-width` | 1280 | webview 窗口宽度 |
| `--webview-height` | 720 | webview 窗口高度 |
| `--webview-resizable` | true | webview 窗口是否可调整大小 |
| `--webview-debug` | false | webview 调试模式 |
| `--basic-port` | 36789 | 系统 Web 服务的监听端口 |
| `--dev-mode` | false | 启用调试模式，显示详细日志且不自动打开 Web 界面 |

## 常见问题

### 1. 编译时提示 "gcc not found"

**解决方案：**
- Windows：安装 MinGW-w64 或 TDM-GCC
- macOS：运行 `xcode-select --install`
- Linux：运行 `sudo apt-get install build-essential`

### 2. 编译时提示 "gtk-3 not found"

**解决方案：**
- Linux：运行 `sudo apt-get install libgtk-3-dev`

### 3. 运行时提示 "webview not supported"

**解决方案：**
- 检查是否使用正确的构建标签 `-tags webview`
- 确认 CGO 已启用 `CGO_ENABLED=1`
- 确认系统支持 webview（Windows、macOS、Linux）

### 4. 窗口无法打开或立即关闭

**解决方案：**
- 检查服务器是否正常启动
- 查看日志输出确认 URL 是否正确
- 尝试使用 `--webview-debug=true` 查看详细错误信息

### 5. HTTPS 证书错误

**解决方案：**
- 确保证书文件路径正确
- 检查证书文件是否存在：`local_data/certs/localhost.pem`
- 检查私钥文件是否存在：`local_data/certs/localhost-key.pem`

## 性能优化

### 减小可执行文件大小

```bash
go build -tags webview -ldflags="-s -w -H windowsgui" -o output.exe main.go
```

- `-H windowsgui`（仅 Windows）：隐藏控制台窗口

### 编译优化

```bash
go build -tags webview -ldflags="-s -w" -gcflags="-l=4" -o output.exe main.go
```

- `-gcflags="-l=4"`：减少内联，减小二进制大小

## 调试技巧

### 启用详细日志

```bash
./Lunar-Astral-Agents-webview --use-webview=true --webview-debug=true
```

### 查看构建信息

```bash
go version -m Lunar-Astral-Agents-webview.exe
```

### 分析依赖

```bash
go list -json -tags webview . | grep '"Imports"'
```

## 发布建议

1. **测试**：在目标平台上充分测试
2. **依赖检查**：确保所有运行时依赖都已安装
3. **签名**：对可执行文件进行数字签名（Windows）
4. **打包**：考虑使用安装程序打包应用
5. **文档**：提供详细的用户文档

## 技术支持

如遇到问题，请检查：
1. Go 版本是否满足要求（1.21+）
2. CGO 是否正确配置
3. 系统依赖是否完整安装
4. 构建命令是否正确

## 相关资源

- [Go CGO 文档](https://pkg.go.dev/cmd/cgo)
- [Webview Go 库](https://github.com/webview/webview)
- [MinGW-w64 官网](https://www.mingw-w64.org/)
- [GTK3 文档](https://www.gtk.org/docs/)
