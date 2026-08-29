# 子系统——网页前端（browser_client）

> 📚 代码级文档参见 [Code Wiki 04·公共子系统](../../docs/code-wiki/04-公共子系统.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

桌面 WebView 窗口管理与本地网络 IP 自动发现模块，负责在桌面上启动嵌入式浏览器窗口。

---

## 功能概述

`browser_client` 提供两种浏览器启动方式，作为宿主程序用来打开前端页面的入口。

| 方式 | 说明 | 适用场景 |
|------|------|----------|
| WebView 嵌入式窗口 | 创建桌面原生窗口，内嵌 Web 界面 | 以原生窗口形态展示本地前端 |
| 系统浏览器回退 | WebView 不可用时自动回退到系统默认浏览器 | WebView 环境缺失时的兜底方案 |

附加能力：

- **本地 IP 自动发现**：自动选择最优局域网 IP（优先 `192.168.x.x` 网段）用于拼接访问地址
- **WebView 单例管理**：确保同一时刻只存在一个 WebView 窗口

---

## 运行与使用方式

`browser_client` 作为库被宿主程序（钛宇-月华、钛宇-琉璃）调用，在需要展示前端页面时由其打开 WebView 窗口，不可用时自动回退系统浏览器。入口函数、IP 发现机制与各调用方式见 [Code Wiki 04 §4.2](../../docs/code-wiki/04-公共子系统.md)，此处不重复。

---

## 依赖

- **WebView2 Runtime**：WebView 嵌入式窗口依赖（Windows 10/11 通常已预装）
- **系统默认浏览器**：WebView 不可用时作为回退目标

---

## 常见问题

### Q: WebView 窗口无法启动怎么办？

1. 确认已安装 [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)（Windows 10/11 通常已预装）
2. 检查通过配置子系统设置的 WebView 窗口尺寸是否合法
3. 如 WebView 不可用，系统会自动回退到系统浏览器

### Q: 如何修改 WebView 窗口样式？

通过 [general_config 子系统](../general_config/README.md) 中的 WebView 参数调整窗口标题、大小、可调整性等属性。

除基础属性外，Windows 平台还支持设置**窗口图标**与**标题栏样式**：

| 参数 | 说明 | 适用系统 |
|------|------|----------|
| `-webview-icon` | 窗口图标 `.ico` 文件路径（空则不设置，使用默认图标） | Win10/11 |
| `-webview-caption-color` | 标题栏背景色，格式 `#RRGGBB` | Win11 |
| `-webview-border-color` | 窗口边框色，格式 `#RRGGBB` | Win11 |
| `-webview-dark-titlebar` | 标题栏深色模式开关 | Win10 1809+/Win11 |

相对图标路径按「可执行文件所在目录 → 当前工作目录」顺序解析（如 `crystal_astral/icon.ico` 相对仓库根目录下的 exe 可直接命中）。加载失败会在日志中输出实际尝试的路径。

示例：`.\Lunar_Astral.exe -webview-icon icon.ico -webview-caption-color "#202124" -webview-dark-titlebar`

### Q: 如何强制使用系统浏览器？

入口函数会自动检测 WebView 支持情况并回退到系统浏览器，无需手动干预。也可通过 [general_config 子系统](../general_config/README.md) 的命令行参数禁用浏览器。

---

## 相关文档

- [项目主文档](../../README.md) —— 环境要求与整体架构
- [配置管理子系统](../general_config/README.md) —— WebView 窗口参数配置
- [钛宇-月华](../../lunar_astral/README.md) —— browser 的主要使用方
- [钛宇-琉璃](../../crystal_astral/README.md) —— browser 的使用方