# 项目开发提示词模板（玻璃风格 / 多文件规范）

## 整体视觉风格（固定不变）

- 所有页面元素统一采用 **半透明模糊玻璃风格**：
  - 卡片、面板等容器：浅色半透明背景（如 `rgba(255,255,255,0.25)`） + `backdrop-filter: blur(12px)`。
  - 按钮、输入框等交互元素：深色半透明背景（如 `rgba(0,0,0,0.35)`），文字为白色。
- 页面背景使用全屏固定的网络图片 背景图片的URL：`/background`

## 技术规范（必须遵循）

- 使用 **HTML5 + CSS3 + 原生 JavaScript (ES6+)**，无任何第三方框架或库。
- CSS 布局采用 Flexbox 或 Grid，确保响应式与跨设备兼容。
- **多文件输出强制规范**：
  - 禁止将所有代码嵌入一个 HTML 文件中。
  - 必须分离为 `index.html`、`styles.css`、`script.js`，以及可能的 `utils.js` 或 `api.js` 等模块文件。
- 代码注释清晰，模块职责分明，变量/函数命名语义化。

## 项目功能需求

1. 编写一个接受ws消息推送并使用气泡的形式渲染到页面的功能。
2. 这个页面只需要处理消息渲染，不需要处理将消息发送给ai的功能。
3. 页面下方应该有一个可以纵向扩展的输入框和发送键，用户可以在输入框中输入消息并点击发送键发送。
4. 输入框应该支持换行输入，用户可以在输入框中输入多行消息。
5. 用户发送的消息也将以气泡的形式渲染到页面上。
6. 页面上的气泡应该支持点击事件，用户可以点击气泡来触发一些操作，比如复制消息、删除消息等。
7. 页面应支持文件拖拽, 当用户将文本文件或图片文件拖拽到页面上时，页面应该能够自动识别并渲染文件内容。如果文件是文本文件，页面应该能够将文件内容渲染到页面上。如果文件是图片文件，页面应该能够将图片渲染到页面上。
8. 页面上的图片应该支持点击预览功能。用户可以点击图片来预览图片的详细内容。
9. 这个页面应支持(Markdown格式/HTML格式/ECharts JSON图表/Mermaid图表/图片)的消息渲染。

## 可选的项目依赖
<!-- Markdown解析 - marked.js -->
<script src="/read/resources/external/marked.min.js"></script>
<!-- 图表库 - ECharts v5.4.3 -->
<script src="/read/resources/external/echarts.min.js"></script>
<!-- 流程图库 - Mermaid v10.5.0 -->
<script src="/read/resources/external/mermaid.min.js"></script>
<!-- 图标库 - Font Awesome 6.4.0 -->
<link rel="stylesheet" href="/read/resources/external/fontAwesome/css/all.min.css">
<!-- 代码高亮 - highlight.js v11.7.0 -->
<script src="/read/resources/external/highlight/highlight.min.js"></script>
<!-- 代码高亮样式 - GitHub风格 -->
<link rel="stylesheet" href="/read/resources/external/highlight/styles/github.css">
<!-- 引入Live2D -->
<script src="/read/resources/external/live2dcubismcore.min.js"></script>
<!-- 引入Pixi.js -->
<script src="/read/resources/external/pixi.5.3.12.min.js"></script>
<!-- 引入Pixi-Live2D -->
<script src="/read/resources/external/pixi-live2d-display-cubism4.min.js"></script>
<!-- 引入QRCode.js -->
<script src="/read/resources/external/qrcode.min.js"></script>
<!-- 引入KaTeX CSS样式文件，为数学公式提供渲染样式 -->
<link rel="stylesheet" href="/read/resources/external/katex/katex.min.css">
<!-- 引入KaTeX核心JS文件，提供数学公式渲染功能 -->
<script defer src="/read/resources/external/katex/katex.min.js"></script>
<!-- 引入KaTeX自动渲染插件，可自动识别并渲染页面中的数学公式 -->
<script defer src="/read/resources/external/katex/contrib/auto-render.min.js"></script>
<!-- 引入多媒体预览样式 -->
<link rel="stylesheet" href="/read/resources/universal/multimedia_preview.css">
<!-- 引入多媒体预览脚本 -->
<script src="/read/resources/universal/multimedia_preview.js"></script>

## 接口（API / 数据）定义

- **端点示例**  
  `WebSocket ws://<host>/ws` ->  示例: `ws://localhost:36797/ws`
```json
// 文本响应
{ "type": "context", "data": { "type": "response", "content": "..."(字符串内容) } }
// 图片响应
{ "type": "image", "data": { "type": "image", "images": ["base64..."] } }
```

## 注意事项（通用质量要求）

1. 所有交互元素必须符合 **无障碍基础规范**（键盘可访问、语义化标签）。
2. 页面布局需适配分辨率（1540×1050、移动端竖屏）。
3. 避免内存泄漏（及时移除事件监听、定时器），保证页面长时间运行稳定。
4. 玻璃模糊效果在低性能设备上可做降级（通过 `@supports` 检测）。
5. 代码中不得包含 `alert`、`confirm` 等阻塞式弹窗（除非业务明确要求）。

## 输出文件结构示意

```
project/
├── index.html
├── styles.css
└── script.js/
```

> 请严格按此结构生成完整的代码文件，保持代码可独立运行。