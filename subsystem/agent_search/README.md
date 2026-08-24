# 子系统——智能网络检索（agent_search）

> 📚 代码级文档参见 [Code Wiki 05·独立AI引擎与运维工具](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，入口 [Code Wiki 门户](../../docs/code-wiki/README.md)。

基于 Chromedp 的 AI 驱动网络搜索智能体，为「月华」提供多引擎搜索、页面内容提取、AI 摘要、相关性判定与向量记忆存储能力。

---

## 功能概述

`agent_search` 是一个完整的搜索智能体，通过 Chromedp 控制 Edge/Chrome 浏览器执行网络搜索，结合多模态 LLM 与嵌入模型进行内容理解、摘要与相关性判断。

| 功能 | 说明 |
|------|------|
| 多引擎搜索 | Bing / 百度 / 搜狗三级降级链，自动切换 |
| 关键词/实体提取 | AI 从查询中提取核心实体与关键词数组，拼接初始查询 |
| 页面内容提取 | DOM 文本优先，文本不足时滚动截图的多模态混合提取 |
| AI 智能摘要 | 调用多模态模型对文本/截图进行中文摘要 |
| 统一搜索模式 | 单一流程：标题初筛 → Top10 提取 → 嵌入打分 → 综合判定 |
| 增强搜索重试 | 首轮无法解答时推测意图换词再搜一轮 |
| 记忆系统 | 结果与失败经验存入向量记忆库，历史查询直接复用 |
| 浏览器健康监控 | CPU / 内存追踪，异常自动重启 |
| 字典网站过滤 | 自动过滤字典/词典类网站，节省 token |
| 工具站兜底检测 | 识别 Bing 快递/物流/在线工具等兜底页，避免空转 |

---

## 工作流程

搜索智能体按 **12 步统一搜索流程** 执行，全程 AI 驱动，不再区分快速/深度两种模式：

1. **实体与关键词提取**：AI 从查询中提取核心实体（独立专名）与关键词数组
2. **初始查询拼接**：关键词去重后以空格拼接成初始查询语句
3. **向量嵌入**：对初始查询调用嵌入模型，得到查询向量
4. **记忆库检索**：用初始查询检索历史记录，高相似度且非失败记录则直接复用
5. **网络搜索判定**：无高匹配 / 时效性需求 / 用户明确要求搜索时进入网络搜索
6. **标题初筛**：用核心实体过滤无关标题（分隔符归一化 + token 化匹配）
7. **逐页混合提取**：Top10 页面先文本、不足截图 → 摘要 → 嵌入打分 + 实体命中判定
8. **综合判定**：将有效摘要拼接，调用 AI 一次性判定能否解答 → 生成报告并入记忆
9. **增强搜索**：无法解答时推测用户真实意图换词再跑一轮
10. **兜底返回**：仍失败返回「月华不知道」并记录失败经验

**关键设计**：
- 相关性判定**不过度保守**——页面能部分解答即视为有效，避免「有答案却说不知道」。
- **失败记录不作为答案复用**，仅当作下次搜索的避坑经验。
- 单页超过 10 秒打不开直接跳过，**不反复重启浏览器**；浏览器重启只用于资源异常（内存/CPU 超限）或引擎页故障场景。

---

## 使用方式

模型配置从 `lunar_config.json` 的 `general_config` 模块读取（搜索多模态模型、嵌入模型的 URL/模型名/API Key）。

### 命令行测试

```powershell
cd d:\Lunar_Astral_Agents\subsystem\agent_search\cmd\search_test
go run .
```

### 作为 Go 库集成

```go
import AgentSearch "LunarSubsystem/AgentSearch"

// 1. 初始化（必须先于 Search）
cfg := AgentSearch.DefaultSearchConfig()
cfg.MaxContextTokens = 16384
if err := AgentSearch.InitSearch(cfg); err != nil {
    // 处理初始化失败
}

// 2. 执行搜索（阻塞、串行）
report, err := AgentSearch.Search("查询南京南站是哪里，有哪些便捷的交通接驳")
// report.Answer       — AI 生成的答案
// report.UsedSources  — 引用来源 URL
// report.FromMemory   — 是否直接来自记忆库
// report.SearchRounds — 实际搜索轮次
```

### 关键配置项

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `MaxContextTokens` | 16384 | 单次 AI 调用最大上下文 tokens |
| `MemoryDBDir` | `local_data/database/memory` | 向量记忆库存储目录 |

### 关键流程常量（variable.go）

| 常量 | 默认值 | 说明 |
|------|--------|------|
| `SingleSearchResults` | 10 | 每轮进入搜索引擎并提取的 TopN 链接 |
| `MaxScreenshotsPerPage` | 6 | 单页最大截图数（分页滚动） |
| `TextHeavyThreshold` | 500 | 文本密集型判定阈值（字符数） |
| `EmbedRelevanceThreshold` | 0.5 | 摘要与查询嵌入余弦相似度阈值 |
| `MemorySimilarityMin` | 0.55 | 记忆库相似度准入阈值 |
| `MemoryDirectAnswerMin` | 0.72 | 直接复用记忆答案的最低相似度 |
| `PageFastSkipTimeout` | 10s | 单页提取超时，超时直接跳过 |
| `BrowserMaxMemMB` | 4096 | 浏览器内存上限，超标触发重启 |

> 作为 Go 库的完整集成方式、函数签名与详细配置项表见 [Code Wiki 05 §5.1](../../docs/code-wiki/05-独立AI引擎与运维工具.md)，此处不重复。

---

## 相关文档

- [配置管理子系统](../general_config/README.md) —— 模型配置与 API 地址
- [文件管理子系统](../file_manager/README.md) —— 记忆库存储后端
- [项目架构文档](../../ARCHITECTURE.md) —— 整体架构