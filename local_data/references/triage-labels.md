# Triage Labels

技能里提到的分诊角色，在本项目统一映射为下面五个标签。标签写在 `spec.md` 顶部的 `Status:` 行（目录约定见 [issue-tracker.md](issue-tracker.md)）。

| 标签 | 含义 | 何时使用 |
|------|------|----------|
| `needs-triage` | 待评估 | 刚记录的想法或问题，还没确认要不要做 |
| `needs-info` | 等待补充 | 缺少关键信息（复现步骤、目标目录、期望行为） |
| `ready-for-agent` | 可交付智能体执行 | 规格已写清目标、改动面与验收方式，可直接开工 |
| `ready-for-human` | 需人工实现 | 涉及真机验证、账号授权、模型文件或线上配置 |
| `wontfix` | 不处理 | 已评估后明确不做，`spec.md` 里写明原因 |

用法：

- 技能提到「打上 AFK-ready 标签」时，写 `Status: ready-for-agent`
- 状态只能从 `needs-triage` 单向推进到 `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`；回到 `needs-triage` 需在 `## 讨论` 里记录原因
- 标签是给人与智能体共用的过滤依据，不要自造同义词
