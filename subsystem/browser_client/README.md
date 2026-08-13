# 子系统——网页前端（browser_client）

桌面 WebView 窗口管理与本地网络 IP 自动发现模块，负责在桌面上启动嵌入式浏览器窗口。

---

## 目录

- [功能概述](#功能概述)
- [核心模块说明](#核心模块说明)
- [接口定义](#接口定义)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`browser_client` 模块提供两种浏览器启动方式：

| 方式 | 说明 |
|------|------|
| **WebView 嵌入式窗口** | 使用 `webview/webview_go` 创建桌面原生窗口，内嵌 Web 界面 |
| **系统浏览器回退** | 当 WebView 不可用时，自动回退到系统默认浏览器 |

附加功能：

- **本地 IP 自动发现**：智能选择最优局域网 IP（优先 192.168.x.x）
- **WebView 单例管理**：确保同一时间只有一个 WebView 实例

---

## 核心模块说明

### 文件职责

| 文件 | 职责 |
|------|------|
| [type.go](type.go) | 核心类型定义（IP 候选结构） |
| [variable.go](variable.go) | 全局状态（单例 WebView + 互斥锁 + 通道） |
| [execute.go](execute.go) | IP 发现逻辑 + 浏览器入口函数 |
| [webView.go](webView.go) | WebView 嵌入式窗口创建与管理 |

### 启动决策树

```
OpenBrowser(url)
    │
    ├── IsWebViewSupported() == true
    │   └── StartWebViewBrowser(url)  ← 嵌入式 WebView
    │       ├── runtime.LockOSThread()   ← GUI 线程绑定
    │       ├── waitForServer(url)       ← TCP 轮询等待后端就绪
    │       ├── createWebView()          ← 单例创建
    │       └── w.Run()                  ← 阻塞事件循环
    │
    └── IsWebViewSupported() == false
        └── OpenSystemBrowser(url)     ← 系统浏览器
```

### IP 发现机制

函数 `GetLocalIP(preferredNetworks []string)` 自动获取最佳局域网 IP：

```
遍历所有网络接口
    │
    ├── 过滤阶段：
    │   ├── 跳过虚拟接口（vEthernet、Hyper-V、Docker、VirtualBox、VPN）
    │   ├── 跳过未启用的接口（FlagUp == 0）
    │   └── 跳过环回接口（FlagLoopback）
    │
    ├── 优先级排序：
    │   ├── 用户指定网段 → 最高优先级
    │   ├── 192.168.x.x   → 次优先
    │   ├── 10.x.x.x       → 第三
    │   ├── 172.16-31.x.x  → 第四
    │   ├── 169.254.x.x    → 链路本地（第五）
    │   └── 其他公网地址   → 最低
    │
    └── 短路优化：找到最高优先级 IP 立即返回
```

### WebView 单例管理

```go
// 全局状态（variable.go）
var (
    webviewMutex       sync.Mutex       // 互斥锁保护共享状态
    webviewInstance    webview.WebView  // 单例实例
    webviewRunning     bool             // 防重入标记
    webviewClosedCh    chan struct{}    // 外部关闭通知通道
    webviewCleanupDone chan struct{}    // 清理完成信号（同步重开）
)
```

- **单例模式**：`createWebView()` 确保只创建一个 WebView 实例
- **线程安全**：`webviewMutex` 保护所有共享状态
- **安全关闭**：`CloseWebView()` 可从任意 goroutine 安全调用

---

## 接口定义

### 导出函数

| 函数 | 说明 |
|------|------|
| `OpenBrowser(url string)` | 入口函数：优先 WebView，回退系统浏览器 |
| `OpenSystemBrowser(url string)` | 跨平台系统浏览器启动 |
| `StartWebViewBrowser(url string)` | WebView 启动（需在专用 goroutine 中调用） |
| `CloseWebView()` | 安全关闭 WebView 窗口 |
| `WebViewClosed() chan struct{}` | 返回关闭通知通道（用于等待窗口关闭） |
| `IsWebViewSupported() bool` | 检查平台是否支持 WebView |
| `IsWebViewRunning() bool` | 检查 WebView 是否正在运行 |
| `GetLocalIP(preferredNetworks []string) (string, error)` | 获取最佳本地 IP |

### 使用示例

```go
package main

import "LunarSubsystem/BrowserClient"

func main() {
    // 方式 1：简单打开 URL
    BrowserClient.OpenBrowser("http://localhost:36789")

    // 方式 2：获取局域网 IP 后打开
    ip, err := BrowserClient.GetLocalIP([]string{})
    if err != nil {
        panic(err)
    }
    url := fmt.Sprintf("http://%s:36789", ip)
    BrowserClient.OpenBrowser(url)

    // 方式 3：等待 WebView 关闭
    go func() {
        BrowserClient.StartWebViewBrowser("http://localhost:36789")
    }()
    <-BrowserClient.WebViewClosed()
    fmt.Println("WebView 已关闭")
}
```

---

## 常见问题

### Q: WebView 窗口无法启动怎么办？

1. 确认已安装 [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)（Windows 10/11 通常已预装）
2. 检查 `config.WebViewWidth` 和 `config.WebViewHeight` 是否合法
3. 如 WebView 不可用，系统会自动回退到系统浏览器

### Q: 如何修改 WebView 窗口样式？

通过 [general_config 子系统](../general_config/README.md) 中的 WebView 参数调整窗口标题、大小、可调整性等属性。

### Q: IP 发现返回了错误的 IP 怎么办？

可以通过 `GetLocalIP` 的 `preferredNetworks` 参数指定优先网段：

```go
// 优先返回 10.0.0.x 网段的 IP
ip, _ := BrowserClient.GetLocalIP([]string{"10.0.0."})
```

### Q: 如何强制使用系统浏览器？

`OpenBrowser()` 内部会自动检测 WebView 支持情况并回退到系统浏览器，无需手动干预。也可通过 [general_config 子系统](../general_config/README.md) 的 `-allow-browser=false` 命令行参数禁用浏览器。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— WebView 窗口参数配置
- [星图·月华](../../lunar_astral/README.md) —— browser 的主要使用方
- [星图·琉璃](../../crystal_astral/README.md) —— browser 的使用方