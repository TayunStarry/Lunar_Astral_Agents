# 网络检索子系统（websearch）

基于 Go 的智能网络检索子系统，提供**轻量摘要**、**网页搜索**、**深度研究**三级检索策略。通过多引擎（Bing + DuckDuckGo）HTML 抓取、网页内容获取、LLM 智能总结与子问题拆解等流水线技术，将互联网信息高效提炼为结构化的搜索结果。

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
| **轻量摘要** | `simple` | Bing 优先 → DuckDuckGo 回退，HTML 解析提取标题/摘要/URL |
| **网页搜索** | `webpage` | 轻量摘要 + 网页内容抓取 + 相关性过滤 + LLM 智能总结 |
| **深度研究** | `depth` | LLM 子问题拆解 → 并行轻量摘要搜索 → 跨子问题 URL 去重 → 综合报告生成 |

三层策略递增：

```
轻量摘要                    网页搜索                         深度研究
Bing → DDG               轻量摘要 + 抓取网页正文           拆解子问题（LLM）
  ↓                         ↓                              ↓
格式化输出                相关性过滤                     并行搜索子问题
                          ↓                              ↓
                       LLM 智能总结                   URL 去重合并
                                                        ↓
                                                   综合研究报告（LLM）
```

---

## 项目结构

```
websearch/
├── type.go          # 类型定义：SearchMode、Searcher 接口、Config 配置树、LLM 协议类型
├── variable.go      # 默认配置常量
├── engine.go        # 搜索引擎实现：Bing + DuckDuckGo HTML 抓取与解析
├── simple.go        # 轻量摘要：Bing 优先 → DDG 回退
├── webpage.go       # 网页搜索：搜索→抓取→相关性过滤→LLM 总结
├── depth.go         # 深度研究：子问题拆解→并行搜索→URL去重→综合报告
├── llm.go           # LLM 客户端：OpenAI v1 协议兼容（/chat/completions）
├── format.go        # 输出格式化器：自然语言、截断、LLM 专用格式
├── search.go        # 子系统入口：New()、Search()、便捷函数
├── go.mod           # Go Module 定义
└── go.sum           # 依赖校验
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
   │ SimpleSearcher │     │ WebpageSearcher  │     │  DepthSearcher    │
   │                │     │                  │     │                   │
   │ Bing ──→ DDG  │     │ Simple.searchRaw │     │  decomposeQuery   │
   │     (回退)     │     │        ↓         │     │  (LLM 拆解)       │
   │                │     │  fetchContent    │     │        ↓          │
   └───────┬────────┘     │  (网页正文抓取)  │     │  并行 simple 搜索  │
           │              │        ↓         │     │        ↓          │
           ▼              │  relevanceFilter │     │  URL 去重         │
   ┌────────────────┐     │  (关键词相关性)  │     │        ↓          │
   │ formatResults  │     │        ↓         │     │  generateReport   │
   └────────────────┘     │  LLM.summarize  │     │  (LLM 综合报告)   │
                          │  (智能总结)      │     └───────────────────┘
                          └──────────────────┘
```

### 搜索引擎层

```
          ┌──────────────┐
          │   Searcher   │  ← 接口
          │   Interface  │
          └──────┬───────┘
                 │
     ┌───────────┴───────────┐
     ▼                       ▼
┌─────────────┐     ┌─────────────────┐
│ BingSearcher│     │DuckDuckGoSearcher│
│             │     │                  │
│ bing.com    │     │ lite.duckduckgo │
│ b_algo 解析 │     │ .com/lite/      │
│ HTML 遍历   │     │ result-snippet  │
└─────────────┘     └─────────────────┘
```

---

## 模块详解

### `type.go` — 类型体系

| 类型 | 说明 |
|------|------|
| `SearchMode` | 搜索模式枚举：`simple` / `webpage` / `depth` |
| `SearchResult` | 单条搜索结果：Title、URL、Snippet |
| `Searcher` | 搜索引擎接口：`Search(query, limit)` + `Name()` |
| `Config` | 完整配置树（轻量摘要/网页搜索/深度研究/LLM/HTTP 子配置） |
| `ChatMessage` / `ChatRequest` / `ChatResponse` | OpenAI v1 协议数据结构 |
| `Provider` | LLM 提供者接口：`Chat(messages) → (text, error)` |

**配置树嵌套结构**：

```
Config
├── Simple:   SimpleConfig   (MaxResults)
├── Webpage:  WebpageConfig  (MaxResults, FetchContent, FetchTimeout, MaxContentLength)
├── Depth:    DepthConfig    (MaxResults, MaxSubQueries)
├── LLM:      LLMConfig      (BaseURL, APIKey, Model, MaxTokens, Temperature)
└── HTTP:     HTTPConfig     (Timeout, UserAgent)
```

### `engine.go` — 搜索引擎

| 引擎 | 搜索源 | 解析方式 |
|------|--------|----------|
| `BingSearcher` | `bing.com/search?mkt=zh-CN` | HTML 遍历 `b_algo` → `h2` 标题 / `a` 链接 / `b_caption` 摘要 |
| `DuckDuckGoSearcher` | `lite.duckduckgo.com/lite/` | HTML 遍历 `result-snippet` → `a` 标题+链接 / 文本摘要 |

**Bing 查询优化**：中文短查询（3-10个汉字）自动加双引号精确匹配，提升中文搜索质量。

**通用工具**：
- `extractTextContent()` — 从 HTML 中提取正文（跳过 script/style/nav/footer/header）
- `truncateText()` — 按 Unicode 字符截断

### `simple.go` — 轻量摘要搜索器

**策略**：Bing 优先 → 失败时自动回退到 DuckDuckGo。

| 方法 | 返回 | 说明 |
|------|------|------|
| `Search(query)` | 格式化文本 | Bing → DDG 回退，返回自然语言格式 |
| `SearchRaw(query)` | `[]SearchResult` | 返回原始结构化数据，供网页搜索/深度研究复用 |

### `webpage.go` — 网页搜索器

**三级流水线**：

1. **搜索** → 调用 `SimpleSearcher.SearchRaw()` 获取原始结果
2. **抓取** → HTTP GET 每个结果 URL，`extractTextContent()` 提取正文（1MB 上限）
3. **过滤** → `checkContentRelevance()` 三级关键词匹配判定：
   - 完整查询词匹配标题/摘要 → 直接通过
   - 任意关键词匹配标题/摘要 → 通过
   - 仅内容匹配 → 需 ≥1 个关键词
4. **总结** → LLM 基于搜索结果生成结构化总结（总结 → 分点 → 来源）

**token 预算控制**：

| 预算项 | 限额 | 说明 |
|--------|------|------|
| 最大抓取条数 | 30 | 网页搜索最多抓取网页数 |
| 总内容预算 | 8000 字符 | 所有搜索内容总字符上限 |
| 单页截断 | 1500 字符 | 每个网页最多保留字符数 |
| LLM 输出上限 | 1500 字符 | LLM 回复最大字符数 |
| Prompt 预算 | 8000 字符 | LLM Prompt 总大小限制 |

### `depth.go` — 深度研究

**四级流水线**：

1. **拆解** → LLM 将用户问题拆解为 3-6 个互补子问题（JSON 数组格式）
2. **并行搜索** → goroutine 并发执行每个子问题的轻量摘要搜索
3. **去重** → 跨子问题 URL 去重（trim 尾部 `/` 后标准化对比）
4. **报告生成** → LLM 汇总所有子问题结果，生成结构化研究报告（核心发现 → 详细分析 → 信息来源）

**预算控制**：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| 每个子问题最多结果 | 15 | 单子问题搜索结果上限 |
| Snippet 截断 | 150 字符 | 注入报告时的摘要截断 |
| Prompt 总预算 | 12000 字符 | 报告生成 prompt 预算 |
| 报告输出上限 | 3000 字符 | LLM 生成报告最大长度 |
| 子问题注入预算 | 8000 字符 | 子问题结果注入 prompt 总量 |

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

### `search.go` — 子系统入口

| 构造函数 | 说明 |
|----------|------|
| `New()` | 默认配置，LLM 功能不可用（仅轻量摘要搜索） |
| `NewWithConfig(cfg)` | 自定义完整配置 |
| `NewWithLLM(cfg, provider)` | 注入自定义 LLM Provider |

| 搜索方法 | 说明 |
|----------|------|
| `Search(query, mode)` | 按模式自动路由到对应搜索器 |
| `SimpleSearch(query)` | 轻量摘要，无需 LLM |
| `WebpageSearch(query)` | 网页搜索，需 LLM |
| `DepthSearch(query)` | 深度研究，需 LLM |

| 便捷函数 | 说明 |
|----------|------|
| `QuickSearch(query)` | 一行轻量摘要 |
| `QuickWebpageSearch(query, llmCfg)` | 一行网页搜索 |
| `QuickDepthSearch(query, llmCfg)` | 一行深度研究 |

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
| `Depth.MaxResults` | 10 | 深度研究单子问题结果数 |
| `Depth.MaxSubQueries` | 6 | 最大子问题数量 |
| `LLM.BaseURL` | `https://api.openai.com/v1` | LLM API 地址（可替换为本地服务） |
| `LLM.Model` | `gpt-4o-mini` | 默认模型 |
| `LLM.MaxTokens` | 4096 | 最大输出 token |
| `LLM.Temperature` | 0.7 | 生成温度 |
| `HTTP.Timeout` | 10s | HTTP 请求超时 |
| `HTTP.UserAgent` | Chrome 120 / Win10 | 搜索引擎请求 UA |

---

## API 接口

### Go 库调用

```go
import "websearch"

// 1. 一行轻量摘要搜索（无需 LLM）
result, err := websearch.QuickSearch("Go 语言最新版本")

// 2. 网页搜索（需 LLM）
llmCfg := websearch.LLMConfig{
    BaseURL: "http://localhost:1234/v1",
    Model:   "qwen2.5-7b-instruct",
    APIKey:  "not-needed",
}
result, err := websearch.QuickWebpageSearch("量子计算最新进展", llmCfg)

// 3. 深度研究（需 LLM）
result, err := websearch.QuickDepthSearch("AI 在医疗领域的应用", llmCfg)

// 4. 自定义配置
cfg := websearch.DefaultConfig()
cfg.LLM = llmCfg
sys := websearch.NewWithConfig(cfg)
sys.SetSimpleMaxResults(15)
result, err := sys.Search("你的问题", websearch.ModeWebpage)
```

### 便捷函数对比

| 函数 | 需要 LLM | 返回格式 |
|------|----------|----------|
| `QuickSearch(query)` | 否 | 自然语言列表 |
| `QuickWebpageSearch(query, llmCfg)` | 是 | LLM 总结 + 来源 |
| `QuickDepthSearch(query, llmCfg)` | 是 | 结构化研究报告 |

---

## 依赖关系

### Go Module 依赖

```
websearch
  └── golang.org/x/net  v0.40.0  (HTML 解析)
```

### 跨模块调用关系

```
lunar_astral / crystal_astral
        │
        ▼
┌───────────────┐
│   websearch   │  ← 网络检索子系统（库级调用）
│               │
│  ├─ engine.go │  → HTTP GET  Bing / DuckDuckGo（外部网络）
│  ├─ webpage.go│  → HTTP GET  搜索结果网页（外部网络）
│  └─ llm.go    │  → HTTP POST LLM API（本地或远程 /chat/completions）
└───────────────┘
```

- **被调用方**：`lunar_astral` 和 `crystal_astral` 通过 Go import 直接引入
- **无 DB 依赖**：纯无状态网络检索，不使用 SQLite 或文件存储
- **外部依赖**：仅 `golang.org/x/net` 用于 HTML 解析

---

## 使用示例

### 场景一：AI 对话中实时搜索

```go
// 月华对话中，用户问实时信息
searchSys := websearch.NewWithConfig(cfg)
result, _ := searchSys.WebpageSearch("今天北京的天气怎么样")
// → LLM 总结后的结构化回答，带来源引用
```

### 场景二：知识深度研究

```go
// 琉璃文件管理场景，用户需要多方信息对比
result, _ := searchSys.DepthSearch("Go vs Rust 在系统编程中的优劣比较")
// → 子问题拆解 → 并行搜索 → URL去重 → 综合研究报告（含核心发现、详细分析、来源列表）
```

### 场景三：轻量级快速查询

```go
// 只需简单搜索结果的场景
result, _ := websearch.QuickSearch("golang.org/x/net 最新版本")
// → 纯搜索列表，无 LLM 开销
```
