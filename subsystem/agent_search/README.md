# 子系统——智能网络检索（agent_search）

基于 Chromedp 的 AI 驱动网络搜索智能体，为月华提供多引擎搜索、页面内容提取、AI 摘要与记忆存储能力。

---

## 功能概述

`agent_search` 子系统是一个完整的搜索智能体，通过 Chromedp 控制浏览器执行网络搜索，结合 AI 模型进行内容理解与摘要。

| 功能 | 说明 |
|------|------|
| 多引擎搜索 | Bing / 百度 / 搜狗三级降级链，自动切换 |
| 页面内容提取 | DOM 文本清洗 + 视觉截图双模式提取 |
| AI 智能摘要 | 调用多模态模型对页面内容进行中文摘要 |
| 双搜索模式 | 快速视觉搜索（截图优先）+ 深度文本搜索 |
| 记忆系统 | 搜索结果存入向量记忆库，历史查询直接复用 |
| 浏览器健康监控 | CPU / 内存追踪，异常自动重启 |
| 字典网站过滤 | 自动过滤字典/词典类网站，节省 token |

---

## 搜索流水线

搜索智能体执行 5 阶段流水线，全程 AI 驱动：

```
Phase 1  记忆检索 → 查询向量记忆库，判断历史答案是否可直接复用
Phase 1.5 AI 模式判定 → 自动选择快速视觉搜索或深度文本搜索
Phase 2  初始搜索 → AI 生成关键词 → 多引擎搜索 → 页面提取 → 摘要
Phase 2.5 信息评估 → AI 判断当前收集的信息是否足以回答
Phase 3  深度搜索 → 多轮补充搜索，AI 生成新角度关键词，嵌入向量去重
Phase 4  报告生成 → AI 整合所有摘要，生成结构化搜索报告
Phase 5  记忆存储 → 搜索结果以自然语言文本存入记忆库
```

### 快速搜索模式

当 AI 判定查询适合视觉搜索时（如产品外观、设计参考、UI 对比），采用纯截图流程：
- 跳过 DOM 文本提取，仅获取页面滚动截图
- 调用多模态模型直接基于截图生成视觉摘要
- 单轮搜索，跳过深度搜索阶段

---

## 项目结构

| 文件 | 职责 |
|------|------|
| `agent.go` | 搜索流水线主控编排，5 阶段流程控制 |
| `ai.go` | AI 调用层，OpenAI 兼容 API（关键词生成、内容摘要、报告生成、嵌入向量去重） |
| `browser.go` | 浏览器生命周期管理、搜索引擎 HTML 解析、DOM 文本清洗 |
| `config.go` | 默认配置与校验 |
| `memory.go` | 记忆系统集成，向量检索与存储 |
| `monitor.go` | 浏览器健康监控（CPU / 内存 / 查询计数） |
| `screenshot.go` | 页面分页滚动截图 |
| `type.go` | 所有类型定义（SearchAgent、SearchReport、PageContent 等） |
| `variable.go` | 全局常量、变量、钩子注册 |

---

## 核心架构

### 依赖关系

```
agent_search
  ├── general_config    ← 模型配置（多模态模型 URL、嵌入模型 URL、API Key）
  ├── file_manager      ← 记忆库存储（向量检索 + 文本存储）
  └── logger_general    ← 彩色终端日志输出
```

### 钩子注册机制

`ai.go` 和 `memory.go` 在 `init()` 阶段自动注册函数钩子到 `agent.go` 的全局变量中，实现关注点分离：
- AI 调用层（`ai.go`）注册：关键词生成、内容摘要、记忆判定、报告生成、搜索模式判定
- 记忆系统（`memory.go`）注册：集合初始化、记忆检索、记忆存储

---

## 使用方式

### 作为 Go 库集成

```go
import "LunarSubsystem/AgentSearch"

// 初始化搜索智能体（模型配置从 lunar_config.json 读取）
cfg := AgentSearch.DefaultSearchConfig()
if err := AgentSearch.InitSearch(cfg); err != nil {
    log.Fatal(err)
}
defer AgentSearch.CloseBrowser()

// 执行搜索（阻塞，串行执行）
report, err := AgentSearch.Search("Go 语言最新版本有哪些特性？")
if err != nil {
    log.Fatal(err)
}
fmt.Println(report.Answer)
```

### 命令行测试

```powershell
cd d:\Lunar_Astral_Agents\subsystem\agent_search\cmd\search_test
go run .
```

---

## 配置说明

搜索智能体的模型配置从 `lunar_config.json` 的 `general_config` 模块读取：

| 配置项 | 说明 |
|--------|------|
| `SearchMultimodalURL` | 多模态模型 API 地址 |
| `SearchMultimodalModel` | 多模态模型名称 |
| `SearchMultimodalKey` | API 密钥 |
| `SearchEmbeddingURL` | 嵌入模型 API 地址 |
| `SearchEmbeddingModel` | 嵌入模型名称 |
| `SearchEmbeddingKey` | 嵌入模型 API 密钥 |

---

## 相关文档

- [配置管理子系统](../general_config/README.md) —— 模型配置与 API 地址
- [文件管理子系统](../file_manager/README.md) —— 记忆库存储后端
- [项目架构文档](../../ARCHITECTURE.md) —— 整体架构