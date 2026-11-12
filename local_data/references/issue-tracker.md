# Issue tracker: Local Markdown

本项目的需求、规格与任务清单都以 markdown 形式放在 `.scratch/`，不接入 GitHub Issues。

## 目录约定

- 一个主题一个目录：`.scratch/specs/<YYYYMMDD>-<slug>/`
- 规格：`.scratch/specs/<YYYYMMDD>-<slug>/spec.md`，从「目标」写起，必须包含「已确认的用户决策」与「已核实的引用关系（改动面）」两节
- 任务：`.scratch/specs/<YYYYMMDD>-<slug>/tasks.md`，按 `01-<slug>` 编号，可勾选
- 状态：每个 `spec.md` 顶部写一行 `Status: <标签>`，标签取值见 [triage-labels.md](triage-labels.md)
- 讨论：追加到文件末尾 `## 讨论` 下，按时间倒序，保留结论而不是过程流水

## 一次性产物

探针脚本、截图、日志、临时跑批数据放 `.scratch/<probe>/`，不需要写成 issue，也不要提交进仓库文档。探针脚本遵守项目语言约定：Go / PowerShell / JavaScript / C++，**禁止 Python**。

## 当技能说「发布到 issue tracker」

新建 `.scratch/specs/<YYYYMMDD>-<slug>/spec.md`（目录不存在就创建），首行写 `Status:`，正文补齐目标、用户决策、改动面三节。任务拆分写同目录 `tasks.md`。

## 当技能说「取回相关工单」

按引用路径直接读该文件；用户通常会给出路径或主题名。主题名对应 `specs/` 下的目录名。
