# 子系统——彩色终端日志（logger）

轻量级彩色终端日志输出模块，提供分级日志格式化与 ANSI 颜色渲染，为所有子系统提供统一的日志输出规范。

---

## 目录

- [功能概述](#功能概述)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [API 接口定义](#api-接口定义)
- [使用示例](#使用示例)
- [常见问题](#常见问题)

---

## 功能概述

`logger` 模块是星月智能平台的**基础设施层**，为所有子系统提供统一的彩色终端日志输出。

| 特性 | 说明 |
|------|------|
| 分级日志 | Info / Warn / Error / Fatal 四级输出 |
| ANSI 彩色 | 不同级别使用不同颜色，终端可读性高 |
| 开发模式 | Info 级别仅在开发模式下输出，生产环境零噪音 |
| 子模块支持 | 每条日志标注模块名 + 子模块名，快速定位来源 |
| 线程安全 | `sync.RWMutex` 保护开发模式标志，并发安全 |
| 零外部依赖 | 仅依赖 Go 标准库 |

### 日志级别与颜色

| 级别 | 颜色 | 输出条件 | 说明 |
|------|------|---------|------|
| **Info** | 青色 (`\033[36m`) | 仅开发模式 | 调试信息，生产环境自动静默 |
| **Warn** | 黄色 (`\033[33m`) | 始终输出 | 警告信息，非致命异常 |
| **Error** | 红色 (`\033[31m`) | 始终输出 | 错误信息，需要关注 |
| **Fatal** | 粗体红 (`\033[1;31m`) | 始终输出 | 致命错误，输出后调用 `os.Exit(1)` |

---

## 项目结构

<div style="font-family: 'Cascadia Code', 'SF Mono', Consolas, monospace; font-size: 0.9em; line-height: 1.6;">
  <ul style="list-style-type: none; padding-left: 0;">
    <li><strong>logger/</strong></li>
    <li style="padding-left: 1.5em;"><code>logger.go</code> <span style="color: #6a737d;">— 彩色终端日志输出（分级日志 + ANSI 颜色 + 开发模式控制）</span></li>
    <li style="padding-left: 1.5em;"><code>go.mod</code> <span style="color: #6a737d;">— Go 模块定义（零外部依赖）</span></li>
  </ul>
</div>

---

## 核心架构

### 日志输出格式

```
[模块名] → 消息内容                        ← Info（青色）
[模块名][WARN] → 消息内容                   ← Warn（黄色）
[模块名][ERROR] → 消息内容                  ← Error（红色）
[模块名][FATAL] → 消息内容                  ← Fatal（粗体红，随后退出）
```

带子模块的格式：

```
[模块名]-[子模块名] → 消息内容              ← SubInfo
[模块名]-[子模块名][WARN] → 消息内容         ← SubWarn
[模块名]-[子模块名][ERROR] → 消息内容        ← SubError
```

### 开发模式控制

```
SetDevMode(true)   ← 开启开发模式
    │
    ├── Info / SubInfo → 输出到终端 ✓
    ├── Warn / SubWarn → 输出到终端 ✓
    ├── Error / SubError → 输出到终端 ✓
    └── Fatal → 输出到终端 + os.Exit(1) ✓

SetDevMode(false)  ← 关闭开发模式（默认）
    │
    ├── Info / SubInfo → 静默 ✗
    ├── Warn / SubWarn → 输出到终端 ✓
    ├── Error / SubError → 输出到终端 ✓
    └── Fatal → 输出到终端 + os.Exit(1) ✓
```

---

## API 接口定义

### 配置函数

| 函数 | 说明 |
|------|------|
| `SetDevMode(v bool)` | 设置开发模式开关（Info 级别仅在开发模式下输出） |
| `SetOutput(w *os.File)` | 重定向日志输出到指定文件 |

### 日志输出函数

| 函数 | 签名 | 级别 | 说明 |
|------|------|------|------|
| `Info` | `Info(module, format string, v ...interface{})` | Info | 普通信息（仅开发模式） |
| `SubInfo` | `SubInfo(module, sub, format string, v ...interface{})` | Info | 带子模块的普通信息 |
| `Warn` | `Warn(module, format string, v ...interface{})` | Warn | 警告信息 |
| `SubWarn` | `SubWarn(module, sub, format string, v ...interface{})` | Warn | 带子模块的警告信息 |
| `Error` | `Error(module, format string, v ...interface{})` | Error | 错误信息 |
| `SubError` | `SubError(module, sub, format string, v ...interface{})` | Error | 带子模块的错误信息 |
| `Fatal` | `Fatal(module, format string, v ...interface{})` | Fatal | 致命错误（输出后退出进程） |

### 参数说明

| 参数 | 类型 | 说明 |
|------|------|------|
| `module` | `string` | 模块名称，如 `"ProxySvr"`、`"VolumeArchive"` |
| `sub` | `string` | 子模块名称，如 `"Cert"`、`"Create"` |
| `format` | `string` | `fmt.Sprintf` 格式字符串 |
| `v` | `...interface{}` | 格式化参数 |

---

## 使用示例

### 基本使用

```go
package main

import "logger"

func main() {
    // 启用开发模式（通常从 config 读取）
    logger.SetDevMode(true)

    // 各级别日志输出
    logger.Info("MyModule", "服务启动于端口 %d", 8080)
    logger.Warn("MyModule", "配置项缺失，使用默认值: %s", "timeout")
    logger.Error("MyModule", "连接失败: %v", err)
    logger.Fatal("MyModule", "无法加载模型: %v", err)  // 输出后 os.Exit(1)
}
```

### 带子模块的日志

```go
logger.SubInfo("ProxySvr", "Cert", "从磁盘加载证书: %s", certPath)
logger.SubWarn("ProxySvr", "TLS", "证书即将过期: %s", expiryDate)
logger.SubError("ProxySvr", "WebSocket", "Hijack 失败: %v", err)
```

### 与 config 子系统集成

```go
// 在程序入口处，根据配置设置开发模式
logger.SetDevMode(*config.Developer)
```

### 重定向日志到文件

```go
f, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
if err != nil {
    panic(err)
}
defer f.Close()
logger.SetOutput(f)
```

---

## 常见问题

### Q: 为什么 Info 日志没有输出？

Info 级别日志仅在开发模式下输出。请确认已调用 `logger.SetDevMode(true)`。在星月智能平台中，开发模式由 [config 子系统](../config/README.md) 的 `Developer` 标志控制。

### Q: 日志颜色在 Windows 终端中不显示？

Windows 10+ 的 Windows Terminal 和 PowerShell 7+ 默认支持 ANSI 颜色。如使用旧版 cmd.exe，可能需要启用 ANSI 转义序列支持。

### Q: 如何在项目中引入 logger？

在 `go.mod` 中添加本地依赖：

```go
require logger v0.0.0

replace logger => ../logger
```

### Q: Fatal 日志输出后程序会退出吗？

是的，`Fatal()` 调用 `os.Exit(1)` 终止进程。如需记录错误但不退出，请使用 `Error()`。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../config/README.md) —— Developer 开发模式配置
- [HTTPS 代理子系统](../proxy/README.md) —— logger 的使用方
- [GGUF 元数据查看器](../gguf_metadata_viewer/README.md) —— logger 的使用方
- [分卷归档子系统](../volume_archive/README.md) —— logger 的使用方
