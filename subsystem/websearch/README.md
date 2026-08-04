# 网络检索子系统（websearch）

三级搜索策略：**轻量摘要** → **网页搜索** → **深度研究**，多引擎回退 + LLM 总结 + 知识库缓存。

## 搜索模式

| 模式 | 标识 | 说明 |
|------|------|------|
| 轻量摘要 | `simple` | 多引擎 HTML 解析，返回标题+摘要+URL，不调用 LLM |
| 网页搜索 | `webpage` | 轻量摘要 + 网页正文抓取 + SPA 浏览器渲染 + 相关性过滤 + LLM 总结 |
| 深度研究 | `depth` | LLM 拆解子问题 → 并行搜索 → URL 去重 → 内容抓取 → 研究校验 → 补充搜索 → 综合报告 |

## 项目结构

```
websearch/
├── type.go          # 类型定义：SearchMode、Searcher 接口、Config、LLM 协议类型
├── variable.go      # 默认配置
├── engine.go        # 搜索引擎：Bing、百度、搜狗、DuckDuckGo HTML 解析
├── simple.go        # 轻量摘要：Bing → 百度 → 搜狗 → DDG 回退
├── webpage.go       # 网页搜索：搜索→抓取→SPA渲染→过滤→LLM总结
├── depth.go         # 深度研究：子问题拆解→并行搜索→URL去重→域名发现→内容抓取
├── gapcheck.go      # 研究校验：信息缺口检测→补充搜索→循环→综合报告
├── knowledge.go     # SQLite 知识库：查询缓存 + URL 内容缓存，三级降级匹配
├── browser.go       # 无头浏览器渲染：SPA 页面交互、链接提取、时效排序
├── search.go        # 子系统入口：NewWithLLM()、Search()、知识库集成
├── format.go        # 输出格式化
├── link.go          # 链接处理：URL 提取→分类→内容抓取→摘要替换
├── authority.go     # 站点权威性评分
├── rerank.go        # 向量相似度重排序
├── llm.go           # OpenAI v1 协议 LLM 客户端
├── retry.go         # HTTP 重试（指数退避）
├── health.go        # 搜索引擎健康检查
├── utils.go         # 工具函数
└── *_test.go        # 测试文件
```

## 核心模块

### 搜索引擎（engine.go）
Bing → 百度 → 搜狗 → DuckDuckGo 依次尝试，百度支持 CAPTCHA 检测后浏览器回退。搜狗支持跳转链接解析。

### 知识库（knowledge.go）
SQLite 持久化，独立文件 `data/search_knowledge.db`。查询采用三级降级：精确匹配 → FTS5 模糊匹配 → 关键词匹配。区分定义类（永久有效）和更新类查询（1小时过期），URL 内容缓存支持 TTL 控制。

### 浏览器渲染（browser.go）
Chrome 无头模式处理 SPA 页面。功能包括：页面交互（点击关键词/导航元素触发内容展开）、同域链接提取、新闻链接时效排序、深度渲染子页面。

### 深度研究（depth.go + gapcheck.go）
- **域名发现**：从搜索结果中动态发现高频域名，推导首页和新闻列表页加入抓取队列（黑名单过滤百科/Wiki/门户）
- **研究校验**：LLM 检测信息缺口 → 补充搜索 → 最多 N 轮循环 → 综合报告
- **URL 缓存**：三级保护（TTL 过期 → 关键词验证 → SPA 壳检测）

### 相关性过滤（webpage.go）
关键词匹配判定，官方网站跳过严格过滤。查询降级：结果不含查询词时用核心实体重搜。

## 配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `Simple.MaxResults` | 10 | 轻量摘要返回数 |
| `Webpage.MaxResults` | 30 | 网页搜索抓取条数 |
| `Webpage.FetchContent` | true | 是否抓取网页正文 |
| `Webpage.FetchTimeout` | 10s | 单页抓取超时 |
| `Depth.Enabled` | false | 是否启用深度搜索 |
| `Depth.MaxSubQueries` | 6 | 最大子问题数 |
| `Depth.MaxGapRounds` | 3 | 研究校验最大轮次 |
| `HTTP.Timeout` | 10s | HTTP 请求超时 |
| `HTTP.MaxRetries` | 2 | 最大重试次数 |

## 使用示例

```go
import "YaraFlow/internal/search"

cfg := websearch.Config{
    Simple:  websearch.SimpleConfig{MaxResults: 10},
    Webpage: websearch.WebpageConfig{MaxResults: 30, FetchContent: true, FetchTimeout: 10},
    Depth:   websearch.DepthConfig{Enabled: true, MaxSubQueries: 6, MaxGapRounds: 3},
    HTTP:    websearch.HTTPConfig{Timeout: 10 * time.Second, UserAgent: "...", MaxRetries: 2},
}

sys := websearch.NewWithLLM(cfg, llmProvider)
sys.SetMemoryProvider(memProvider)
sys.SetDebugLogFunc(func(format string, args ...interface{}) { log.Printf(format, args...) })

// 三种模式
result, _ := sys.Search("天气", websearch.ModeSimple)    // 轻量摘要
result, _ := sys.Search("新闻", websearch.ModeWebpage)   // 网页搜索+LLM总结
result, _ := sys.Search("深度分析", websearch.ModeDepth) // 深度研究
```

## 依赖

- `golang.org/x/net` — HTML 解析
- `github.com/chromedp/chromedp` — 无头浏览器
- `github.com/mattn/go-sqlite3` — SQLite 知识库