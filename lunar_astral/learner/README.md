# Learner 学习者智能体模块

## 概述

Learner 是星月智能系统的**研究型智能体**，采用双记忆架构（知识记忆 + 经验记忆）和 9 步工作流，具备自主推理、记忆检索、网络搜索、深度研究与持续学习能力。

当用户提出需要查证、搜索或研究的问题时，Learner 自动执行完整的研究流程并生成结构化研究报告。

---

## 架构概览

```
┌─────────────────────────────────────────────────────┐
│                    Learner 智能体                     │
├─────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ LLMClient │  │ Search   │  │  MemoryManager   │  │
│  │ (推理引擎) │  │ Manager  │  │ (知识+经验双表)   │  │
│  └────┬─────┘  └────┬─────┘  └────────┬─────────┘  │
│       │             │                 │              │
│  ┌────┴─────────────┴─────────────────┴──────────┐  │
│  │              WorkflowRunner                    │  │
│  │           (9 步研究流程编排)                     │  │
│  └───────────────────────────────────────────────┘  │
│                       │                              │
│  ┌────────────────────┴──────────────────────────┐  │
│  │           PromptTemplates (5 个模板)            │  │
│  │  从 assets/prompts/learner*.md 加载            │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### 文件职责

| 文件 | 职责 |
|------|------|
| `type.go` | 所有类型定义（struct、interface、type alias） |
| `variable.go` | 全局变量、常量、Token 预算预设 |
| `learner.go` | 顶层入口：初始化、执行、Goja 绑定 |
| `llm.go` | LLM 客户端：API 调用、Token 预算控制、JSON 提取 |
| `memory.go` | 记忆管理器：知识/经验双表 CRUD、相似度匹配 |
| `search.go` | 搜索管理器：初步搜索、深度搜索、网页搜索 |
| `prompt.go` | Prompt 模板加载与构建 |
| `workflow.go` | 9 步工作流执行器 |

---

## 快速开始

### 1. 初始化

```go
import "lunar_astral/learner"

cfg := learner.LearnerConfig{
    BaseURL:        "http://localhost:8080/v1",  // LLM API 地址
    APIKey:         "your-api-key",               // API 密钥
    Model:          "system-multimodal",          // 模型名称
    MaxTokens:      4096,                         // 最大输出 token
    Temperature:    0.7,                          // 温度
    EmbeddingURL:   "http://localhost:8080/v1",  // 嵌入服务地址
    EmbeddingKey:   "your-api-key",               // 嵌入服务密钥
    EmbeddingModel: "system-embedding",           // 嵌入模型名称
}

l := learner.NewLearner(cfg)
if err := l.Init(); err != nil {
    // 初始化失败：Prompt 文件缺失、记忆库不可用等
    log.Fatal(err)
}
```

### 2. 执行研究

```go
// 未读消息文本数组（TS 层传入，仅包含触发搜索的最新消息，不再传入对话历史）
unreadMessages := []string{"帮我查一下XXX的最新消息", "还有YYY的情报"}

// 模式选择
report, err := l.Execute(unreadMessages, "full")
if err != nil {
    // 处理错误，Learner 内部已有降级策略
}
fmt.Println(report)
```

### 3. 调试导出

```go
// 导出完整运行时上下文到 JSON 文件，用于排查问题
path, err := l.DumpContext(unreadMessages, "full", "debug.json")
```

---

## 运行模式

| 模式 | 参数值 | 说明 |
|------|--------|------|
| 完整研究 | `"full"`（默认） | 执行完整的 9 步工作流，包含网络搜索和深度搜索 |
| 回忆模式 | `"recall"` | 仅查询推理完善 + 记忆库检索，跳过所有网络搜索 |

### 回忆模式适用场景

- 用户明确表示"回忆一下上次聊过什么"
- 仅需查询本地记忆库，无需联网
- 响应速度更快，延迟更低

---

## 9 步研究工作流

完整研究模式按以下步骤执行，每个步骤有独立的 Token 预算和降级策略：

```
步骤 a: AI 推理完善请求
  └─ 将模糊的用户请求完善为结构化查询（含搜索词建议）
  └─ 降级：LLM 失败时使用原始请求

步骤 b: 查询记忆库（知识 + 经验双表并行）
  └─ 同时查询 learner_knowledge 和 learner_experience
  └─ 降级：记忆库不可用时跳过，不阻断流程

步骤 d: 初步网络搜索
  └─ 使用 search_terms 进行多词搜索，合并去重
  └─ 降级：搜索不可用时跳过

步骤 e: AI 总结评估 + 决策
  └─ 综合记忆结果和搜索摘要，判断信息是否充足
  └─ 充足 → 直接返回阶段性摘要
  └─ 不足 → 进入深度搜索
  └─ 降级：评估失败时强制进入深度搜索

步骤 g/h: 深度搜索循环（最多 5 轮）
  └─ 每轮执行网页搜索 + AI 内容评估
  └─ 信息充足时提前退出
  └─ 自动去重：禁止搜索相同或高度相似的查询词
  └─ 降级：单轮失败继续下一轮，全部失败构建部分报告

步骤 i: 统一处理工作流
  └─ 更新知识记忆（LLM 提取 → 回退模板提取）
  └─ 更新经验记忆（策略描述）
  └─ 替代高相似度旧知识记忆（相似度 ≥ 0.75）
  └─ 返回最终研究报告
```

---

## 双记忆架构

### 知识记忆（`learner_knowledge`）

- **用途**：存储从网络搜索获取的事实性知识、数据、信息
- **查询**：默认返回 Top 10，相似度阈值 ≥ 0.60
- **更新**：研究完成后自动提取新知识条目并批量写入
- **替代**：新记忆与旧记忆相似度 ≥ 0.75 时自动替换

### 经验记忆（`learner_experience`）

- **用途**：存储请求处理策略与搜索指导经验
- **查询**：默认返回 Top 5
- **内容**：包含查询类型、搜索策略、搜索轮次、效果评估
- **作用**：在步骤 e 评估时作为上下文注入（策略 B），辅助决策

### 降级决策

当知识库可用但记忆查询结果不足时（匹配数 < 2 或相似度 < 0.60），Learner 返回"月华不知道"而非低质量回答，确保输出可靠性。

---

## Token 预算体系

每个工作流步骤有独立的 Token 预算，防止单步消耗过多资源：

| 步骤 | 预算变量 | 输入上限 | 输出上限 |
|------|----------|----------|----------|
| 推理完善 | `BudgetRefine` | 2,000 | 800 |
| 策略评估 | `BudgetEvaluate` | 8,000 | 2,000 |
| 搜索评估 | `BudgetSearchEval` | 12,000 | 3,000 |
| 报告生成 | `BudgetReport` | 10,000 | 4,000 |
| 记忆更新 | `BudgetMemoryUpdate` | 4,000 | 800 |

全局上下文上限：**16,384 tokens**，其中输出预留 4,096 tokens。

---

## Goja（JS 运行时）绑定

Learner 通过以下 4 个函数暴露给 TS 层的 Goja 运行时：

### `learnerInit(baseURL, apiKey, model, maxTokens, temperature, embeddingURL, embeddingKey, embeddingModel)`

初始化学习者实例。返回 `[true, null]` 或 `[false, error]`。

```javascript
const [ok, err] = learnerInit(
    "http://localhost:8080/v1",
    "sk-xxx",
    "system-multimodal",
    4096,
    0.7,
    "http://localhost:8080/v1",
    "sk-xxx",
    "system-embedding"
);
```

### `learnerExecute(unreadMessages, mode?)`

执行研究流程。`unreadMessages` 为字符串数组，`mode` 可选，默认为 `"full"`。返回 `[report, error]`。

```javascript
const [report, err] = learnerExecute(
    ["帮我查一下XXX的最新消息", "还有YYY的情报"],
    "full"  // 或 "recall"
);
```

### `learnerIsReady()`

检查学习者是否已初始化。返回 `bool`。

### `learnerDumpContext(unreadMessages, mode?, outputPath?)`

导出运行时上下文到 JSON 文件（覆写模式）。`unreadMessages` 为字符串数组。返回 `[path, error]`。

```javascript
const [path, err] = learnerDumpContext(
    ["帮我查一下XXX"],
    "full",
    "debug.json"
);
```

---

## 搜索子系统

### 三种搜索模式

| 模式 | 常量 | 说明 |
|------|------|------|
| 简单搜索 | `AgentModeSimple` | 轻量摘要，返回标题+URL+摘要 |
| 网页搜索 | `AgentModeWebpage` | 搜索 + 网页内容抓取 + LLM 智能总结 |
| 深度研究 | `AgentModeDepth` | 多轮深度搜索，适合复杂研究 |

### 深度搜索特性

- 最多 5 轮，每轮搜索条件必须不同
- 自动去重：完全相同或高度相似（差异 < 30%）的查询词被跳过
- 优先使用 refine 步骤生成的 `search_terms`，用尽后使用 LLM 补充词
- 每轮结果通过 AI 评估，信息充足时提前退出

---

## 配置常量参考

| 常量 | 值 | 说明 |
|------|-----|------|
| `MaxDeepSearchRounds` | 5 | 深度搜索最大轮次 |
| `MemoryQueryTopK` | 10 | 知识记忆查询返回条数 |
| `ExperienceQueryTopK` | 5 | 经验记忆查询返回条数 |
| `SimpleSearchMaxResults` | 10 | 初步搜索最大结果数 |
| `DeepSearchResultMaxChars` | 8,000 | 深度搜索结果压缩阈值 |
| `ContextMaxTokens` | 16,384 | 单次 LLM 调用上下文上限 |
| `CharPerToken` | 1.5 | 中文字符/Token 估算比率 |
| `KnowledgeMinSimilarity` | 0.60 | 知识记忆最低相似度 |
| `KnowledgeMinMatchCount` | 2 | 知识记忆最少匹配条数 |
| `MemoryUpdateSimilarityThreshold` | 0.75 | 记忆替代相似度阈值 |
| `MinReportLength` | 10 | 报告最小有效字符数 |

---

## Prompt 模板

Learner 依赖 5 个 Prompt 模板文件，必须内嵌在 `assets/prompts/` 目录中：

| 文件 | 用途 | 对应步骤 |
|------|------|----------|
| `learnerRefine.md` | 查询推理完善 | 步骤 a |
| `learnerEvaluate.md` | 策略评估 | 步骤 e |
| `learnerSearchEval.md` | 搜索内容评估 | 步骤 h |
| `learnerMemory.md` | 记忆更新 | 步骤 i |
| `learnerReport.md` | 报告生成 | 步骤 i |

**重要**：所有模板文件必须存在，加载失败时 Learner 初始化将直接报错，不会回退到内置默认值。

---

## 研究报告格式

Learner 生成的报告以 `[研究报告]` 为标识头，包含以下结构：

```
[研究报告]

## 研究主题
（用户查询的完善版本）

## 研究结论
（基于所有收集信息的核心结论）

## 支持证据
（标注来源：网络 / 记忆）

## 疑点与未解决问题
（如有）

## 研究方法说明
（本研究采用的方法流程）
```

---

## 错误处理与降级策略

Learner 采用**多层降级**策略，确保在各种异常情况下仍能返回有意义的结果：

| 异常场景 | 降级行为 |
|----------|----------|
| Prompt 文件缺失 | 初始化失败，直接报错（不降级） |
| 记忆库不可用 | 跳过记忆查询，仅使用搜索 |
| 搜索子系统不可用 | 跳过搜索，仅使用记忆库 |
| 查询推理失败 | 使用原始查询作为完善后查询 |
| 策略评估失败 | 强制进入深度搜索 |
| 深度搜索失败 | 基于已有信息构建降级报告 |
| 报告验证失败（乱码/过短） | 尝试使用知识库数据构建替代报告 |
| 知识库匹配不足 | 返回"月华不知道"而非低质量回答 |

---

## 线程安全

Learner 实例通过 `sync.RWMutex` 保护，支持并发安全访问：

- `learnerInit`：写锁
- `learnerExecute`：读锁
- `learnerIsReady`：读锁

注意：同一时间只允许一个 Learner 实例存在，重新初始化会先关闭旧实例。