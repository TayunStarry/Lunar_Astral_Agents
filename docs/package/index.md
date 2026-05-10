# 扩展包文档

> 📦 **扩展包**是星月智能的可复用Web组件库，为前端界面提供丰富的UI组件和交互功能。

---

## 📁 包目录

| 包名 | 说明 | 文档 |
|------|------|------|
| [archive](archive.md) | 归档管理组件 | 归档历史、许可证信息 |
| [database_manager](database_manager.md) | 数据库管理器 | 数据库管理界面组件 |
| [file_explorer](file_explorer.md) | 文件浏览器 | 文件浏览与管理界面 |
| [image_generation](image_generation.md) | 图像生成组件 | AI绘图界面与提示词管理 |
| [message_rendering](message_rendering.md) | 消息渲染组件 | 消息展示与渲染 |
| [multimedia_preview](multimedia_preview.md) | 多媒体预览组件 | 音视频内容预览 |
| [parameter_assistant](parameter_assistant.md) | 参数助手组件 | 参数配置辅助界面 |
| [screenshot_manager](screenshot_manager.md) | 截图管理器 | 屏幕截图管理界面 |

---

## 🔧 公共依赖

### 样式库

| 库 | 说明 | 版本 |
|---|---|---|
| **FontAwesome** | 图标库 | v6.x |
| **KaTeX** | 数学公式渲染 | 最新版 |
| **Highlight.js** | 代码高亮 | 最新版 |

### 脚本库

| 库 | 说明 |
|---|---|
| **ECharts** | 图表库 |
| **Marked** | Markdown解析 |
| **Mermaid** | 图表渲染 |
| **QRCode** | 二维码生成 |
| **Pixi.js** | 2D图形渲染 |
| **Live2D Cubism** | 虚拟形象渲染 |

---

## 📋 使用方式

### 方式一：CDN引入

```html
<!-- 标准依赖 -->
<link rel="stylesheet" href="/package/standard_dependency/styles.css">
<script src="/package/standard_dependency/script.js"></script>
```

### 方式二：独立引入

```html
<!-- 单独引入需要的包 -->
<link rel="stylesheet" href="/package/fontAwesome/css/all.min.css">
<script src="/package/echarts.min.js"></script>
```

---

## 🔗 关联文档

- [星图·琉璃 文档](../crystal_astral.md)
- [星图·月华 文档](../luna_astral.md)
- [主项目README](../README.md)