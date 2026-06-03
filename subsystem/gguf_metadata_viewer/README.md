# GGUF 元数据查看器

桌面应用工具，通过嵌入式浏览器提供 GGUF 模型文件的拖放式元数据查看功能。

---

## 功能概述

| 功能 | 说明 |
|------|------|
| **文件拖放** | 支持将 .gguf 文件直接拖入窗口或点击选择 |
| **元数据解析** | 解析 GGUF 二进制格式的所有键值对 |
| **前端展示** | 摘要卡片 + 可搜索过滤的完整元数据表 |
| **终端日志** | 元数据同步输出到控制台（使用 config 子系统控制） |
| **嵌入式窗口** | 基于 WebView2 的原生桌面窗口 |

---

## 项目结构

```
gguf_metadata_viewer/
├── main.go                 # 入口：启动HTTP服务 + 嵌入式浏览器
├── go.mod / go.sum         # Go 模块定义
├── gguf/
│   └── decode.go           # GGUF 二进制格式解析（参考 llama/metadata）
├── server/
│   ├── server.go           # HTTP服务器 + API端点 + 元数据终端打印
│   └── static/
│       ├── index.html      # 前端界面（拖放区域 + 结果展示）
│       ├── style.css       # 深色主题样式
│       └── script.js       # 拖放交互 + API调用 + 搜索过滤
└── README.md               # 本文档
```

### 依赖关系

```
main.go
 ├── config  (../config)   — 配置管理（Developer模式、端口等）
 ├── logger  (../logger)   — 彩色终端日志
 ├── browser (../browser)  — 嵌入式 WebView2 窗口
 └── server/server.go
      └── gguf/decode.go   — GGUF 二进制解析
```

---

## 环境要求

| 组件 | 版本/说明 |
|------|----------|
| **Go** | ≥ 1.24（需支持泛型和 embed 包） |
| **操作系统** | Windows 10/11、macOS、Linux |
| **WebView2** | Windows 10/11 通常已预装；[手动安装地址](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) |
| **CGO** | 需启用（webview 依赖） |
| **GCC** | Windows 下推荐使用 [MSYS2 MinGW64](https://www.msys2.org/) 或 Zig |

---

## 构建

### 一键构建（推荐）

```powershell
# 环境检查 → 依赖下载 → 编译，一键完成
.\build.ps1

# 清理缓存后重建
.\build.ps1 -Clean

# 交叉编译 Linux 版本
.\build.ps1 -TargetOS linux -TargetArch amd64
```

构建脚本会自动：
1. 检查 Go + GCC 环境
2. 执行 `go mod tidy` 下载依赖
3. 编译（含 `-s -w` 剥离符号，二进制约 6MB）

### 手动构建

```powershell
cd d:\Lunar_Astral_Agents\subsystem\gguf_metadata_viewer

$env:CGO_ENABLED = "1"
$env:CC = "gcc"

go mod tidy
go build -ldflags="-s -w" -o gguf_metadata_viewer.exe .
```

---

## 运行

### 基本运行

```powershell
# 直接运行（使用默认配置）
.\gguf_metadata_viewer.exe

# 默认信息：
#   HTTP 端口: 36889 (config.BasicPort + 100)
#   窗口标题: 使用 config.WebViewTitle 配置
#   开发者模式: 关闭（终端日志仅显示 WARN/ERROR）
```

### 启用详细日志

```powershell
# 终端输出完整的元数据日志（带颜色）
.\gguf_metadata_viewer.exe -developer
```

### 自定义端口

```powershell
# 指定基础端口（实际端口 = 基础端口 + 100）
.\gguf_metadata_viewer.exe -basic-port 37000

# 实际 HTTP 服务端口为: 37000 + 100 = 37100
```

### 更多启动参数

| 参数 | 默认值 | 说明 |
|------|-------|------|
| `-developer` | `false` | 启用开发模式，显示详细日志 |
| `-basic-port` | `36789` | 系统基础端口（实际端口 +100） |
| `-webview-title` | `星月智能 - 月之华` | 窗口标题 |
| `-webview-width` | `345` | 窗口宽度 |
| `-webview-height` | `520` | 窗口高度 |
| `-webview-resizable` | `true` | 窗口是否可调整大小 |

---

## 使用指南

1. **启动程序**：运行 `gguf_metadata_viewer.exe`，弹出嵌入式浏览器窗口
2. **加载模型**：
   - 方式一：从文件管理器将 `.gguf` 文件拖入窗口的虚线区域
   - 方式二：点击"选择文件"按钮，浏览并选择文件
3. **查看结果**：
   - 顶部展示**模型摘要卡片**（名称、架构、量化方式等关键参数）
   - 下方展示**完整元数据表**（所有键值对，按键名排序）
   - 使用搜索框可按关键字过滤元数据条目
4. **终端日志**：使用 `-developer` 模式时，终端同步打印完整的元数据列表
5. **关闭**：关闭 WebView 窗口即可退出程序

---

## 技术细节

### GGUF 流式解析流程

核心设计原则：**仅读取 header，跳过 tensor 数据**。GGUF 文件结构为 `[Header(元数据)] → [Tensor Info] → [Tensor Data]`，元数据部分是完整的键值对集合，解析完即可停止读取。

```
用户拖入 .gguf 文件
    │
    ▼
前端 FormData 上传 → POST /api/upload
    │
    ▼
服务端接收 → gguf.ParseMetadataFromReader(io.Reader)  ← 直接从上传流解析
    │                                            （无需写入磁盘）
    ├── 读取 4B 魔数 (GGUF 0x46554747)
    ├── 检测字节序 (小端/大端)
    ├── 读取 4B 版本号 (v1/v2/v3+)
    ├── 读取 tensorCount + metadataCount
    ├── 仅读取 metadataCount 个键值对 ───┐
    │   ├── 字符串类型                    │  ← header 部分，通常 < 10MB
    │   ├── 数值 (uint8..float64)         │     与模型文件总大小无关
    │   ├── 布尔类型                      │     100MB ~ 30GB 均秒级完成
    │   └── 数组类型 (递归)               │
    └── 返回 map[string]any → 停止读取 ──┘
    │
    ▼
转换为 JSON 友好格式 → 返回前端
同时打印到终端日志（Developer 模式）
```

> **关键优势**：20GB 的 GGUF 文件与 200MB 的文件解析速度完全相同，因为只读取 header。无需磁盘暂存，无需等待完整上传，无文件大小限制。

### 元数据摘要提取

程序自动从原始元数据中提取以下关键信息生成摘要卡片：

| 搜索键 | 摘字段 | 说明 |
|--------|--------|------|
| `general.name` | 模型名称 | 模型的显示名称 |
| `general.architecture` | 架构 | llama / qwen2 / gemma 等 |
| `general.file_type` | 量化方式 | 如 Q4_K_M |
| `general.quantization_version` | 量化版本 | 量化格式版本号 |
| `*.context_length` | 上下文长度 | 支持的最大 token 数 |
| `*.embedding_length` | 嵌入维度 | 隐藏层维度 |
| `*.block_count` | 层数 | Transformer 层数 |
| `*.attention.head_count` | 注意力头数 | 多头注意力数量 |
| `*.attention.head_count_kv` | KV头数 | KV 缓存注意力头数 |
| `*.feed_forward_length` | FFN维度 | 前馈网络维度 |
| `tokenizer.ggml.token_count` | 词表大小 | 词汇表 token 数量 |

> 注：带 `*` 的键会自动匹配 `llama.*` 和 `qwen2.*` 等不同架构前缀。

### 架构设计原则

- **流式解析**：`ParseMetadataFromReader(io.Reader)` 接受任意数据源（文件/网络流/管道），仅读取 header，跳过 tensor data
- **无文件大小限制**：解析性能与模型文件大小无关，100MB 到 30GB 均秒级完成
- **分层清晰**：解析层（gguf/decode.go）仅处理二进制格式，服务层（server/server.go）负责 HTTP 和格式化，前端层（static/）负责交互
- **错误处理**：每层均有完善的错误捕获、日志输出和用户反馈
- **零磁盘开销**：上传文件直接从 multipart 流解析，无需磁盘暂存
- **前后端分离**：前端通过 REST API 与后端通信，可独立测试

---

## 常见问题

### Q: 运行时提示 "无法加载 embedded view"

**原因**：缺少 WebView2 Runtime。

**解决**：
1. Windows 10/11 通常已预装，尝试更新系统
2. 手动下载安装：[WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
3. 如 WebView 不可用，程序会自动回退到系统默认浏览器

### Q: 构建时报 CGO 错误

**原因**：webview 依赖 CGO，需要 GCC 编译器。

**解决**（Windows）：
```powershell
# 使用 MSYS2
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-pkg-config

# 或使用 Zig 作为 C 编译器
$env:CC = "zig cc"
go build -o gguf_metadata_viewer.exe .
```

### Q: 拖放文件后解析失败

**原因**：文件格式不是有效的 GGUF 格式。

**检查**：
1. 确认文件的魔数是 `GGUF`（十六进制 `47 47 55 46`）
2. 文件是否完整（未损坏）
3. 查看终端日志中的具体错误信息

### Q: 如何解析多个模型文件

每次拖放新的文件即可，界面会自动刷新显示当前文件的信息。

---

## 相关文档

- [llama/metadata](../../../%E6%9C%88%E4%B9%8B%E5%8D%8E-%E8%B5%84%E6%96%99%E5%BA%93%E5%AD%98/llama/metadata/) — GGUF 解析参考实现
- [browser 子系统](../browser/README.md) — 嵌入式浏览器模块
- [config 子系统](../config/README.md) — 配置管理模块
- [星月智能项目](../../README.md) — 主项目文档