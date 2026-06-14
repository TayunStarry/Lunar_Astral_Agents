# WebSearch - 网络检索子系统

独立、完整的网络检索子系统，支持浅层搜索、深层搜索（AI增强）和研究搜索三种模式。可无缝嵌入任何 Go 项目。

## 架构概览

```
websearch/
├── type.go          # 所有类型定义（接口、结构体、枚举）
├── variable.go      # 包级变量与默认配置
├── search.go        # 统一入口（System 结构体 + 便捷函数）
├── engine.go        # 搜索引擎实现（Bing、DuckDuckGo、HTML 解析）
├── llm.go           # OpenAI v1 协议 LLM 客户端
├── shallow.go       # 浅层搜索（Bing + DDG 回退）
├── deep.go          # 深层搜索（搜索 + 抓取 + LLM 总结）
├── research.go      # 研究搜索（子问题拆解 + 并行搜索 + 综合报告）
├── format.go        # 结果格式化
├── search_test.go   # 测试
├── go.mod / go.sum  # 独立 Go 模块
└── README.md
```

## 快速开始

### 1. 引入依赖

在 `go.mod` 中添加：

```
require websearch v0.0.0
```

或直接将 `websearch` 目录复制到项目中，替换 module 路径。

### 2. 最简用法 — 浅层搜索

```go
package main

import (
    "fmt"
    "log"
    "websearch"
)

func main() {
    // 使用默认配置创建（无需 AI，仅浅层搜索）
    sys := websearch.New()

    result, err := sys.ShallowSearch("Go 语言并发编程")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result)
}
```

### 3. 深层搜索 — 需要 AI

```go
package main

import (
    "fmt"
    "log"
    "websearch"
)

func main() {
    cfg := websearch.DefaultConfig()
    // 配置 OpenAI v1 协议兼容的 AI 服务
    cfg.LLM.BaseURL = "https://api.openai.com/v1"
    cfg.LLM.APIKey  = "sk-your-api-key"
    cfg.LLM.Model   = "gpt-4o-mini"

    sys := websearch.NewWithConfig(cfg)

    // 深层搜索：搜索 + 网页抓取 + AI 总结
    result, err := sys.DeepSearch("深度学习的核心概念")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result)
}
```

### 4. 研究搜索 — 多维度深度分析

```go
// 研究搜索：自动拆解子问题 → 并行搜索 → AI 综合报告
result, err := sys.ResearchSearch("2024年AI大模型在医疗领域的应用进展与挑战")
```

### 5. 按模式搜索

```go
result, err := sys.Search("查询内容", websearch.ModeShallow)   // 浅层
result, err := sys.Search("查询内容", websearch.ModeDeep)       // 深层
result, err := sys.Search("查询内容", websearch.ModeResearch)   // 研究
```

## 三种搜索模式

| 模式 | 说明 | 需要 AI | 适用场景 |
|------|------|---------|----------|
| **浅层搜索** | Bing + DuckDuckGo 回退，快速获取结果 | 否 | 快速查询、简单信息获取 |
| **深层搜索** | 搜索 + 网页内容抓取 + AI 总结 | 是 | 需要深度分析和总结的查询 |
| **研究搜索** | 子问题拆解 + 并行搜索 + AI 综合报告 | 是 | 复杂多维度研究问题 |

## 配置说明

```go
cfg := websearch.DefaultConfig()

// 浅层搜索配置
cfg.Shallow.MaxResults = 10           // 最大搜索结果数

// 深层搜索配置
cfg.Deep.MaxResults      = 30         // 最大搜索结果数
cfg.Deep.FetchContent    = true       // 是否抓取网页正文
cfg.Deep.FetchTimeout    = 10         // 抓取超时（秒）
cfg.Deep.MaxContentLength = 2000      // 单页最大内容长度（字符）

// 研究搜索配置
cfg.Research.MaxResults    = 10       // 每个子问题最大结果数
cfg.Research.MaxSubQueries = 6        // 最大子问题数量

// AI 配置（遵循 OpenAI v1 协议）
cfg.LLM.BaseURL     = "https://api.openai.com/v1"
cfg.LLM.APIKey      = "sk-xxx"
cfg.LLM.Model       = "gpt-4o-mini"
cfg.LLM.MaxTokens   = 4096
cfg.LLM.Temperature = 0.7

// HTTP 配置
cfg.HTTP.Timeout   = 10 * time.Second
cfg.HTTP.UserAgent = "Mozilla/5.0 ..."
```

## 自定义 LLM 提供者

实现 `websearch.Provider` 接口即可接入任意 AI 服务：

```go
type Provider interface {
    Chat(messages []ChatMessage) (string, error)
}
```

使用方式：

```go
provider := myCustomLLMProvider{}
sys := websearch.NewWithLLM(cfg, provider)
```

## 自定义搜索引擎

实现 `websearch.Searcher` 接口即可扩展搜索引擎：

```go
type Searcher interface {
    Search(query string, limit int) ([]SearchResult, error)
    Name() string
}
```

通过 `websearch.NewShallowSearcherWithEngine` 注入自定义引擎：

```go
bing := myBingSearcher{}
ddg  := myDDGSearcher{}
sh   := websearch.NewShallowSearcherWithEngine(bing, ddg, 10)
```

## 便捷函数

```go
// 快速浅层搜索（默认配置）
result, err := websearch.QuickSearch("查询内容")

// 快速深层搜索
result, err := websearch.QuickDeepSearch("查询内容", websearch.LLMConfig{
    BaseURL: "https://api.openai.com/v1",
    APIKey:  "sk-xxx",
    Model:   "gpt-4o-mini",
})

// 快速研究搜索
result, err := websearch.QuickResearchSearch("查询内容", llmCfg)
```

## 错误处理

所有搜索方法在 AI 不可用时自动降级：

- **深层搜索**：LLM 不可用时，回退到原始搜索结果格式化输出
- **研究搜索**：LLM 拆解失败时，降级为单问题搜索；报告生成失败时，回退到原始汇总
- **浅层搜索**：Bing 失败时自动回退到 DuckDuckGo

## 运行测试

```bash
cd subsystem/websearch
go test ./... -v
```

## 依赖

- `golang.org/x/net` — HTML 解析

## OpenAI v1 协议兼容性

LLM 客户端严格遵循 OpenAI Chat Completions API 规范：

- 端点：`POST {BaseURL}/chat/completions`
- 认证：`Authorization: Bearer {APIKey}`
- 请求体：`{ model, messages, max_tokens, temperature }`
- 响应体：`{ choices: [{ message: { role, content } }] }`

兼容所有 OpenAI v1 协议的服务，包括但不限于：OpenAI、Azure OpenAI、DeepSeek、通义千问、智谱 AI 等。
