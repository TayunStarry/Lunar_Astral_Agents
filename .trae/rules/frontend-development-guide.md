# 星月智能 通用开发技能指南
---

## 一、前端开发规划

### 1.1 技术栈与依赖约束

#### 强制约束

| 层 | 技术 | 说明 |
|----|------|------|
| 语言 | HTML5 + CSS3 + Vanilla JS (ES6+) | **禁止 Python** |
| 模块 | ES Modules (`type="module"`) | 使用 `import`/`export` |
| 样式 | 纯 CSS + CSS Variables | 无预处理器 |
| 图标 | Font Awesome 6.4.0 | `<i class="fas fa-xxx">` |
| 通信 | WebSocket（主）+ Fetch（辅） | 实时双向通信 |

#### 已验证依赖库

| 库 | 用途 |
|----|------|
| marked.js | Markdown 渲染 |
| ECharts 5.4.3 | 数据图表 |
| Mermaid 10.5.0 | 流程图/时序图 |
| highlight.js 11.7.0 | 代码高亮 |
| KaTeX | 数学公式 |
| QRCode.js | 二维码 |

### 1.2 JS 架构：ES Module + 类封装

跨项目共有两种架构模式，按复杂度选择：

**复杂应用** — ES Module 类架构（`subsystem/lunar_harness/clients/lunar`、`subsystem/lunar_harness/clients/crystal`）：
- 每个功能域封装为独立类（`WebSocketClient`、`AudioQueue`）
- 主入口 `App` 类聚合各模块，通过 `DOMContentLoaded` 启动
- 代码分隔符 `// ==== 区块名 ====` 标记功能边界

**简单应用** — 函数式架构（`lunar.math-studio`、`lunar.translate-studio`）：
- 全局 DOM 引用平铺于文件头部（`const xx = document.getElementById('xx')`）
- 公共能力以统一模板提供：深色模式切换（`localStorage` 持久化）、Toast 提示、历史记录（`localStorage` 持久化 + 渲染）
- 代码分隔符 `// ===== 区块名 =====` 标记功能边界，按「DOM 引用 → 主题 → 状态 → 渲染 → 事件 → 消息监听 → 初始化」顺序组织
- 所有函数平铺在同一文件

### 1.3 HTML 结构模式

**统一使用 `html-page-skeleton` 技能**：玻璃拟态骨架模板（背景层 + 玻璃蒙层 + 左右面板 + Toast）、关键约定（标题格式 / `data-page` / 图标按钮）、标准依赖管控规则（按需引用扁平化清单中的正式包、禁止外部依赖、`/reliance/` 内置共享依赖路径）全部以该技能为准。

### 1.4 设计风格

**统一使用 `glassmorphism-design` 技能**：设计令牌（CSS Variables）、深色模式、主题色变体、布局规范、组件类名、CSS 组织方式等全部以该技能为准，禁止自行发明设计系统或硬编码颜色/圆角/阴影/尺寸。

### 1.5 通信模式

跨项目统一的通信架构：

| 通道 | 端点模式 | 用途 |
|------|----------|------|
| WebSocket | `/ws` | 实时消息流、流式响应 |
| HTTP Fetch | `/v1/chat/completions` | OpenAI 兼容 API 调用 |
| HTTP 上传 | `/file/write` | 文件上传（`X-File-Name` 头传路径） |
| HTTP 静态 | `/reliance/` | 内置共享 JS/CSS 库（`/file/read/package/<包名>/` 为扩展包自有资源） |

WebSocket 客户端统一实现：自动重连（最多 5 次，间隔 3 秒）、消息处理器注册模式。

### 1.6 LTPX / AtoA 智能体设计规范

**统一使用 `ltpx-atoa-agent` 技能**：核心设计理念（Agent-to-Agent）、完整调用链路、包工具注册（metadata.json）、智能体实现规范（独立上下文 40 轮 / 工具循环上限 8 次 / 模型配置读取）、mini-LTP 特例全部以该技能为准。

「链」与「智能体」的职责边界是**硬性约定**，见 [agent-chain-boundary.md](agent-chain-boundary.md)：只有 `protocol/agentbase` 能发起推理，链只调用智能体，禁止任何位置自行拼模型请求或解析流式输出。

---

## 二、后端开发规划

### 2.1 Go 代码规范

项目后端采用 Go 语言，遵循严格的**文件职责分离**原则：

| 文件 | 职责 | 强制要求 |
|------|------|----------|
| `type.go` | 所有类型定义（struct、interface、type alias） | **必须**集中存放，禁止在其他文件定义导出类型 |
| `variable.go` | 所有全局变量与常量声明 | **必须**集中存放，禁止与业务逻辑代码混合 |

**目录结构示例**：
```
module/
├── type.go       # 类型定义集中管理
├── variable.go   # 变量/常量集中管理
├── handler.go    # 请求处理逻辑
└── main.go       # 入口与路由注册
```

此规范确保代码结构清晰、职责分明，避免类型或变量散落各文件导致的维护困难。

---

## 三、新模块开发速查

### 前端模块

1. **HTML 骨架**：调用 `html-page-skeleton` 技能（玻璃拟态骨架模板 + 标准依赖管控）
2. **标题**：`『 星月智能 』模块名`
3. **设计风格**：调用 `glassmorphism-design` 技能（设计令牌 / 深色模式 / 主题色 / 组件类名）
4. **JS**：简单用函数式（`// ===== 区块名 =====` 分区），复杂用 ES Module 类架构
5. **语言**：`lang="zh-CN"`，中文注释
6. **命名**：CSS kebab-case，状态类 `.active` / `.visible` / `.hidden`

### 后端模块

1. **类型定义**：统一放在 `type.go` 文件中
2. **变量声明**：统一放在 `variable.go` 文件中
3. **职责分离**：业务逻辑不得与类型/变量定义混在同一文件

---

## 四、代码组织与文件规模约定

单个代码文件尽量满足「不超过 10 个函数」**或**「不超过 300 行」中的至少一条（非硬性标准，按更宽松的一条判断；纯数据/类型文件以行数为准）。超出时按职责拆分，不做机械截断：

1. **按职责分包**：入口文件只保留编排与对外 API，工具定义、纯计算、提示词、持久化等拆到同名子目录
2. **入口路径不变**：保留 `xxx.ts` 作为对外入口，新增 `xxx/` 子目录承载拆分模块，调用方无需改动
3. **禁止目录索引导入**：`rollup` 未启用 node-resolve 插件，`import` 必须写到具体文件名（如 `./painter/tools/index`），不能写 `./painter/tools`
4. **拆分优先级**：先抽纯函数与常量（无副作用），再抽只依赖显式参数的处理器，最后才考虑基类/组合
5. **合并同理**：同一职责的零散小函数应合并回同一模块；不要为了拆而拆，拆分目标是可读性与可定位性
6. **命名分层**：承载独立 Go 包的模块目录用大驼峰（如 `agents/file/FileOperations`、`agents/file/FileEditor`、`executors/WebCrawler`），族根与分组目录用小写（如 `agents/file`、`agents/viewer`、`agents/desktop/execute`）；源文件统一 snake_case，前端 kebab-case，类型名与函数名不动。
