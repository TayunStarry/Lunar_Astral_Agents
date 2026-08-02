# 网络检索子系统（websearch）

基于 Go 的智能网络检索子系统，提供**轻量摘要**、**网页搜索**、**深度研究（大会辩论）**三级检索策略。通过多引擎（Bing → 百度 → 搜狗 → DuckDuckGo）HTML 抓取、网页内容获取、LLM 智能总结与子问题拆解等流水线技术，将互联网信息高效提炼为结构化的搜索结果。

---

## 目录

- [功能概览](#功能概览)
- [项目结构](#项目结构)
- [核心架构](#核心架构)
- [模块详解](#模块详解)
- [配置说明](#配置说明)
- [API 接口](#api-接口)
- [依赖关系](#依赖关系)
- [使用示例](#使用示例)

---

## 功能概览

| 搜索模式 | 标识 | 说明 |
|---------|------|------|
| **轻量摘要** | `simple` | Bing 优先 → 百度 → 搜狗 → DuckDuckGo 回退，HTML 解析提取标题/摘要/URL |
| **网页搜索** | `webpage` | 轻量摘要 + 网页内容抓取 + 相关性过滤 + LLM 智能总结 |
| **深度研究** | `depth` | LLM 子问题拆解 → 并行轻量摘要搜索 → 跨子问题 URL 去重 → 大会辩论 → 综合报告生成 |

三层策略递增：

```
轻量摘要                    网页搜索                         深度研究（大会辩论）
Bing→百度→搜狗→DDG       轻量摘要 + 抓取网页正文           拆解子问题（LLM）
  ↓                         ↓                              ↓
格式化输出                相关性过滤                     并行搜索子问题
                          ↓                              ↓
                       LLM 智能总结                   URL 去重合并
                                                        ↓
                                                   大会辩论（多轮）
                                                        ↓
                                                   综合研究报告（LLM）
```

---

## 项目结构

```
websearch/
├── type.go             # 类型定义：SearchMode、Searcher 接口、Config 配置树、LLM 协议类型
├── variable.go         # 默认配置常量
├── engine.go           # 搜索引擎实现：Bing、DuckDuckGo、百度、搜狗 HTML 抓取与解析
├── simple.go           # 轻量摘要：Bing → 百度 → 搜狗 → DDG 回退
├── webpage.go          # 网页搜索：搜索→抓取→相关性过滤→LLM 总结
├── depth.go            # 深度研究：子问题拆解→并行搜索→URL去重→内容抓取→报告生成
├── llm.go              # LLM 客户端：OpenAI v1 协议兼容（/chat/completions）
├── format.go           # 输出格式化器：自然语言、截断、LLM 专用格式
├── search.go           # 子系统入口：NewWithLLM()、Search()、浏览器懒加载
├── browser.go          # 无头浏览器渲染器：SPA 页面动态内容提取、百度 CAPTCHA 回退
├── link.go             # 链接处理：URL 提取、内容抓取、摘要替换
├── retry.go            # HTTP 重试策略：指数退避、最大重试次数
├── health.go           # 搜索引擎健康检查：可用性检测、故障切换
├── assembly.go         # 大会辩论编排器：辩论状态管理、补充搜索触发
├── assembly_types.go   # 大会辩论类型定义：辩论角色、发言记录、状态结构
├── assembly_delegate.go # 辩论代表：维新派/守旧派角色定义与发言生成
└── *_test.go           # 单元测试文件
```

---

## 核心架构

### 三层搜索流水线

```
                        ┌──────────────────────────┐
                        │        System             │
                        │   Search(query, mode)     │
                        └────────────┬─────────────┘
                                     │
            ┌────────────────────────┼────────────────────────┐
            ▼                        ▼                        ▼
   ┌────────────────┐     ┌──────────────────┐     ┌───────────────────┐
   │ SimpleSearcher │     │ WebpageSearcher  │     │    Assembly       │
   │                │     │                  │     │    (大会辩论)      │
   │ Bing→百度→搜狗 │     │ Simple.searchRaw │     │  DepthSearcher    │
   │   →DDG (回退)  │     │        ↓         │     │  (数据采集)       │
   │                │     │  fetchContent    │     │        ↓          │
   └───────┬────────┘     │  (网页正文抓取)  │     │  辩论编排         │
           │              │        ↓         │     │  (多轮辩论)       │
           ▼              │  relevanceFilter │     │        ↓          │
   ┌────────────────┐     │  (关键词相关性)  │     │  补充搜索         │
   │ formatResults  │     │        ↓         │     │  (按需触发)       │
   └────────────────┘     │  LLM.summarize  │     │        ↓          │
                          │  (智能总结)      │     │  综合报告         │
                          └──────────────────┘     └───────────────────┘
```

### 搜索引擎层

```
          ┌──────────────┐
          │   Searcher   │  ← 接口
          │   Interface  │
          └──────┬───────┘
                 │
     ┌───────────┼───────────┬───────────┐
     ▼           ▼           ▼           ▼
┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐
│ Bing    │ │ Baidu   │ │ Sogou   │ │ DuckDuckGo  │
│         │ │         │ │         │ │             │
│ bing    │ │ baidu   │ │ sogou   │ │ lite.ddg    │
│ .com    │ │ .com    │ │ .com    │ │ .com/lite/  │
└─────────┘ └─────────┘ └─────────┘ └─────────────┘
```

---

## 模块详解

### `type.go` — 类型体系

| 类型 | 说明 |
|------|------|
| `SearchMode` | 搜索模式枚举：`simple` / `webpage` / `depth` |
| `SearchResult` | 单条搜索结果：Title、URL、Snippet、IsOfficial |
| `Searcher` | 搜索引擎接口：`Search(query, limit)` + `SearchRaw(query, limit)` + `Name()` |
| `Config` | 完整配置树（轻量摘要/网页搜索/深度研究/HTTP 子配置） |
| `ChatMessage` / `chatRequest` / `chatResponse` | OpenAI v1 协议数据结构 |
| `Provider` | LLM 提供者接口：`Chat(messages) → (text, error)` |
| `VisionProvider` | 图片识别提供者接口（可选，由调用方注入，用于处理链接中图片的内容分析） |
| `MemoryProvider` | 记忆查询接口（由项目层实现，供大会辩论使用） |

**配置树嵌套结构**：

```
Config
├── Simple:   SimpleConfig   (MaxResults)
├── Webpage:  WebpageConfig  (MaxResults, FetchContent, FetchTimeout, MaxContentLength)
├── Depth:    DepthConfig    (Enabled, MaxRounds, MaxSubQueries, MaxSupplementarySearches)
└── HTTP:     HTTPConfig     (Timeout, UserAgent, MaxRetries, RetryBackoff)
```

### `engine.go` — 搜索引擎

| 引擎 | 搜索源 | 解析方式 |
|------|--------|----------|
| `BaiduSearcher` | `baidu.com/s` | HTML 遍历，支持无头浏览器回退处理 CAPTCHA |
| `SogouSearcher` | `sogou.com/web` | HTML 遍历，支持 window.location.replace 链接解析 |
| `BingSearcher` | `bing.com/search?mkt=zh-CN` | HTML 遍历 `b_algo` → `h2` 标题 / `a` 链接 / `b_caption` 摘要 |
| `DuckDuckGoSearcher` | `lite.duckduckgo.com/lite/` | HTML 遍历 `result-snippet` → `a` 标题+链接 / 文本摘要 |

**通用工具**：
- `extractTextContent()` — 从 HTML 中提取正文（跳过 script/style/nav/footer/header）
- `truncateText()` — 按 Unicode 字符截断

### `simple.go` — 轻量摘要搜索器

**策略**：Bing 优先 → 百度 → 搜狗 → 失败时回退到 DuckDuckGo。

| 方法 | 返回 | 说明 |
|------|------|------|
| `Search(query)` | 格式化文本 | 多引擎回退，返回自然语言格式 |
| `SearchRaw(query, limit)` | `[]SearchResult` | 返回原始结构化数据，供网页搜索/深度研究复用 |

### `webpage.go` — 网页搜索器

**五级流水线**：

1. **搜索** → 调用 `SimpleSearcher.SearchRaw()` 获取原始结果（含官方网站标记）
2. **HTTP抓取** → 并行抓取所有结果 URL，`extractTextContent()` 提取正文（1500字符截断）
3. **SPA渲染** → 对内容不足或关键词不匹配的页面，并行浏览器渲染提取动态内容
4. **过滤** → `checkContentRelevance()` 关键词匹配判定，官方网站跳过严格过滤，SPA渲染失败不杀
5. **总结** → LLM 基于搜索结果生成结构化总结（总结 → 分点 → 来源）

**token 预算控制**：

| 预算项 | 限额 | 说明 |
|--------|------|------|
| 最大抓取条数 | 30 | 网页搜索最多抓取网页数 |
| 总内容预算 | 8000 字符 | 所有搜索内容总字符上限 |
| 单页截断 | 1500 字符 | 每个网页最多保留字符数 |
| LLM 输出上限 | 1500 字符 | LLM 回复最大字符数 |
| Prompt 预算 | 8000 字符 | LLM Prompt 总大小限制 |

### `depth.go` — 深度研究

深度研究由 `DepthSearcher` 负责数据采集，`Assembly` 负责辩论编排，整体流程：

**数据采集阶段**：

1. **拆解** → LLM 将用户问题拆解为子问题（JSON 数组格式），含疑问词优先拆解
2. **并行搜索** → goroutine 并发执行每个子问题的轻量摘要搜索（并发数固定为3）
3. **去重** → 跨子问题 URL 去重（trim 尾部 `/` 后标准化对比）
4. **内容抓取** → 对去重后的结果抓取完整网页正文，支持 SPA 页面浏览器渲染

**大会辩论阶段**（由 Assembly 编排）：

5. **辩论初始化** → 注入研究数据（网络搜索 + 记忆信息），创建四角色辩论代表
6. **多轮辩论** → 维新派 → 守旧派 → 反对者 → 整合者串行发言，每轮可触发补充搜索
7. **补充搜索** → 解析整合者【仍需信息】第一条作为搜索查询，执行补充搜索后继续辩论
8. **报告生成** → 辩论收敛后，LLM 基于完整辩论记录生成结构化研究报告

**预算控制**：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| 每个子问题最多结果 | 15 | 单子问题搜索结果上限 |
| Snippet 截断 | 300 字符 | 注入报告时的摘要截断 |
| Prompt 总预算 | 12000 字符 | 报告生成 prompt 预算 |
| 报告输出上限 | 3000 字符 | LLM 生成报告最大长度 |
| 子问题注入预算 | 8000 字符 | 子问题结果注入 prompt 总量 |

### `browser.go` — 无头浏览器渲染器

用于处理 SPA 页面（单页应用）的动态内容提取：

- **SPA 页面渲染**：使用 Chrome 无头模式加载动态内容，提取 JS 渲染后的链接和正文
- **懒加载**：浏览器不在启动时创建，首次搜索时按需初始化
- **资源管理**：搜索完成后自动关闭浏览器进程，避免内存泄漏
- **独立标签**：每个渲染任务使用独立标签页，防止上下文干扰

### `assembly.go` — 大会辩论编排器

深度搜索的顶层编排者，组织四角色串行多轮辩论：

| 角色 | 立场 | 信息来源 | 职责 |
|------|------|----------|------|
| **维新派** | 积极/建设性 | 网络搜索结果优先 | 从最新网络信息出发，寻找积极信号和发展机遇 |
| **守旧派** | 审慎/批判性 | 记忆库信息优先 | 从历史经验出发，关注风险和局限性，回应维新派论点 |
| **反对者** | 挑刺/质疑 | 完整研究数据 | 指出维新派和守旧派的逻辑漏洞、证据链断裂之处 |
| **整合者** | 综合/收敛判断 | 完整研究数据 | 判断辩论是否收敛，总结核心分歧，列出【仍需信息】 |

**辩论流程**：维新派发言 → 守旧派发言 → 反对者挑刺 → 整合者判断收敛

**补充搜索**：当整合者在【仍需信息】中列出缺失信息时，自动提取第一条作为搜索查询执行补充搜索

**收敛判定**：整合者输出 CONVERGED 或反对者连续两轮无新问题

### `llm.go` — LLM 客户端

`OpenAIProvider` 实现 OpenAI v1 `/chat/completions` 协议：

```
POST {BaseURL}/chat/completions
Authorization: Bearer {APIKey}
Content-Type: application/json

{ "model": "...", "messages": [...], "max_tokens": N, "temperature": T }
```

- 兼容任何 OpenAI v1 协议兼容的 API 端点（如 LM Studio、Ollama、vLLM 等本地推理服务）
- 默认超时 120 秒（适配本地模型推理延迟）

### `format.go` — 输出格式化

| 函数 | 用途 |
|------|------|
| `formatResults()` | 轻量摘要搜索自然语言格式：「标题」：摘要 |
| `formatResultsTruncated()` | 带 Snippet 截断保护（防 prompt 溢出） |
| `formatResultsForLLM()` | LLM 专用格式：编号 + Markdown + 来源 URL |
| `formatWebpageResultsFallback()` | 网页搜索无 LLM 时的 fallback 格式化（4000 字符截断保护） |

### `link.go` — 链接处理

| 方法 | 说明 |
|------|------|
| `ProcessLinks(query)` | 检测并替换消息中的链接为摘要，返回替换后的纯文本和链接描述列表 |

### `search.go` — 子系统入口

| 构造函数 | 说明 |
|----------|------|
| `NewWithLLM(cfg, provider)` | 使用 LLM Provider 创建网络检索子系统 |

| 搜索方法 | 说明 |
|----------|------|
| `Search(query, mode)` | 按模式自动路由到对应搜索器 |

| 设置方法 | 说明 |
|----------|------|
| `SetDebugLogFunc(fn)` | 设置诊断日志回调 |
| `SetMemoryProvider(mp)` | 设置记忆提供者（供大会辩论使用） |
| `SetVisionProvider(vp)` | 设置图片识别提供者（可选，用于分析链接中图片内容） |
| `SetDownloadFunc(fn)` | 设置下载回调函数 |
| `EnsureBrowser()` | 确保浏览器渲染器可用（懒加载） |
| `Close()` | 释放子系统资源（包括浏览器） |

---

## 配置说明

### 默认配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `Simple.MaxResults` | 10 | 轻量摘要返回数 |
| `Webpage.MaxResults` | 30 | 网页搜索抓取条数 |
| `Webpage.FetchContent` | `true` | 是否抓取网页正文 |
| `Webpage.FetchTimeout` | 10s | 单页抓取超时 |
| `Webpage.MaxContentLength` | 2000 | 单页内容上限 |
| `Depth.Enabled` | `true` | 是否启用大会辩论深度搜索 |
| `Depth.MaxRounds` | 1 (变量默认) / 3 (配置默认) | 大会辩论最大轮次 |
| `Depth.MaxSubQueries` | 6 | 最大子问题数量 |
| `Depth.MaxSupplementarySearches` | 3 | 辩论中补充搜索最大次数（0=禁用） |
| `HTTP.Timeout` | 10s | HTTP 请求超时 |
| `HTTP.UserAgent` | Chrome 131 / Win10 | 搜索引擎请求 UA |
| `HTTP.MaxRetries` | 2 | 最大重试次数 |
| `HTTP.RetryBackoff` | 500ms | 基础退避时间 |

> **注意**：`Depth.MaxRounds` 在 `variable.go` 中默认值为 1（用于单元测试），在 `config.go` 中默认值为 3（用于生产环境）。

---

## API 接口

### Go 库调用

```go
import "YaraFlow/internal/search"

// 创建网络检索子系统
cfg := websearch.Config{
    Simple: websearch.SimpleConfig{
        MaxResults: 10,
    },
    Webpage: websearch.WebpageConfig{
        MaxResults:       30,
        FetchContent:     true,
        FetchTimeout:     10,
        MaxContentLength: 2000,
    },
    Depth: websearch.DepthConfig{
        Enabled:                  true,
        MaxRounds:                3,
        MaxSubQueries:            6,
        MaxSupplementarySearches: 3,
    },
    HTTP: websearch.HTTPConfig{
        Timeout:      10 * time.Second,
        UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0",
        MaxRetries:   2,
        RetryBackoff: 500 * time.Millisecond,
    },
}

// 注入 LLM Provider（由项目层提供）
sys := websearch.NewWithLLM(cfg, llmProvider)

// 设置记忆提供者（供大会辩论使用）
sys.SetMemoryProvider(memProvider)

// 设置调试日志
sys.SetDebugLogFunc(func(format string, args ...interface{}) {
    log.Printf(format, args...)
})

// 执行搜索
result, err := sys.Search("AI 在医疗领域的应用", websearch.ModeDepth)
```

### 搜索模式对比

| 模式 | 需要 LLM | 返回格式 |
|------|----------|----------|
| `ModeSimple` | 否 | 自然语言列表 |
| `ModeWebpage` | 是 | LLM 总结 + 来源 |
| `ModeDepth` | 是 | 结构化研究报告（大会辩论） |

---

## 依赖关系

### Go Module 依赖

```
websearch
  ├── golang.org/x/net  v0.40.0  (HTML 解析)
  └── github.com/chromedp/chromedp  (无头浏览器渲染)
```

### 跨模块调用关系

```
processor/tool/builtin_search.go
        │
        ▼
┌───────────────┐
│   websearch   │  ← 网络检索子系统（库级调用）
│               │
│  ├─ engine.go │  → HTTP GET  百度/搜狗/Bing/DDG（外部网络）
│  ├─ webpage.go│  → HTTP GET  搜索结果网页（外部网络）
│  ├─ browser.go│  → Chrome 无头浏览器（SPA 页面渲染）
│  └─ llm.go    │  → HTTP POST LLM API（本地或远程 /chat/completions）
└───────────────┘
```

- **被调用方**：`internal/processor/tool/builtin_search.go` 通过 Go import 直接引入
- **无 DB 依赖**：纯无状态网络检索，不使用 SQLite 或文件存储
- **外部依赖**：`golang.org/x/net` 用于 HTML 解析，`github.com/chromedp/chromedp` 用于浏览器渲染

---

## 使用示例

### 场景一：AI 对话中实时搜索

```go
// 对话中，用户问实时信息
sys := websearch.NewWithLLM(cfg, llmProvider)
result, _ := sys.Search("今天北京的天气怎么样", websearch.ModeWebpage)
// → LLM 总结后的结构化回答，带来源引用
```

### 场景二：知识深度研究（大会辩论）

```go
// 用户需要多方信息对比和深度分析
sys := websearch.NewWithLLM(cfg, llmProvider)
sys.SetMemoryProvider(memProvider)
result, _ := sys.Search("Go vs Rust 在系统编程中的优劣比较", websearch.ModeDepth)
// → 子问题拆解 → 并行搜索 → URL去重 → 大会辩论 → 综合研究报告
```

### 场景三：轻量级快速查询

```go
// 只需简单搜索结果的场景
sys := websearch.NewWithLLM(cfg, nil) // 无需 LLM
result, _ := sys.Search("golang.org/x/net 最新版本", websearch.ModeSimple)
// → 纯搜索列表，无 LLM 开销
```
