# Domain Docs

本文件说明：在本仓库里探索代码前该读什么领域文档，以及写输出时该用哪套术语。

## 探索前先读

| 想知道什么 | 读哪里 |
|------------|--------|
| 系统能做什么、一次输入怎么走完 | [系统阅读指南](../../local_data/references/README.md) 的能力地图 |
| 八条链、四层智能体、工具注册 | [09 调度与智能体分层](../../local_data/references/09-调度与智能体分层.md) |
| 扩展包、形态标签、外部工具边界 | [07 外部工具与扩展包](../../local_data/references/07-外部工具与扩展包.md) |
| 端口、模型配置、安全边界 | [10 运行配置与安全](../../local_data/references/10-运行配置与安全.md) |
| 智能体与链的硬性职责边界 | [agent-chain-boundary.md](../../.trae/rules/agent-chain-boundary.md) |
| 前端技术栈与骨架规范 | [frontend-development-guide.md](../../.trae/rules/frontend-development-guide.md) |

本仓库**没有** `CONTEXT.md` / `CONTEXT-MAP.md` / `docs/adr/`，不要为了「补全结构」而新建它们。历史上代码注释曾指向 `docs/adr/000N-*`，这些死引用已清理；要看设计依据就去 code-wiki 对应篇章。

## 架构决策记在哪里

- 已确认的方案与改动面：`.scratch/specs/<日期>-<主题>/spec.md`（含「已确认的用户决策」段）
- 阶段性设计提案：`.trae/documents/`
- 代码级硬约束：`.trae/rules/`，与提案冲突时以规则为准，并在提案里显式写出冲突点

## 术语约定

输出（issue 标题、重构提案、假设、测试名）必须使用项目既有词汇，不要换同义词：

- **智能体 / 链 / 执行体**：分别对应 `agents/`、`chain/`、`executors/`，不要混用「模块」「任务」泛指
- **月华 / 琉璃**：对话主体 / 工具与承载人格，不要写成「AI」「助手」
- **资源句柄**：形如 `图片1234` 的引用文法，不要写成「资源 ID」
- **形态标签**：`Zero-LTP` / `Node-LTP` / `Mini-LTP` / `Self-LTP` / `LTP9` / `DeepSeek` / `DeepDemos`，顺序与 [07 外部工具与扩展包](../../local_data/references/07-外部工具与扩展包.md) 的标签表一致

术语表入口在 [系统阅读指南](../../local_data/references/README.md) 末尾。要用的概念不在表里时：先确认它是否真是项目语言；确实缺失就记一笔，等 `/domain-modeling` 正式收口。
