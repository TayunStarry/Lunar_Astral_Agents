## 硬性架构约定

### 智能体与链边界

只有 `protocol/agentbase` 能发起模型推理；`chain/`、`engine/`、`server/`、`executors/` 一律不得自行构造模型请求、解析流式输出或持有 `StreamRequest`。链只负责编排与调用智能体。完整约束、自查命令、新增链/智能体清单与豁免清单见 `.trae/rules/agent-chain-boundary.md`。

## Agent skills

### Issue tracker

Issues tracked as local markdown files under `.scratch/`. See `local_data/references/issue-tracker.md`.

### Triage labels

Default label vocabulary (needs-triage / needs-info / ready-for-agent / ready-for-human / wontfix). See `local_data/references/triage-labels.md`.

### Domain docs

领域术语与架构决策的约定见 `local_data/references/domain.md`。
