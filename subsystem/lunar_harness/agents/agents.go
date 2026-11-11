// Package agents 收纳全部智能体：通常一个智能体一个子包，模型推理只允许从这里发起。
//
// 子包一般按 type.go（导出类型）、variable.go（AgentID / 角色 / 超时 / 轮次上限）、
// agent.go（唯一的模型入口）、catalog.go（工具注册与工具目录）组织。新增智能体还须
// 把 AgentID 与显示名登记到 protocol/agentinfo，并在 app/tui_hooks.go 的
// roleAgentIDs 与 buildToolCatalog 挂上，否则推理实况里看不到它、故障也修不到它。
//
// 有四个子包不是智能体，按用途分两类：
//   - 发起模型调用、但只作为共用基座：toolagent（工具型智能体的通用工具循环）；
//   - 不发起模型调用、只提供原子能力：filesupport（文件版本存储）、
//     ArchiveOps（归档写操作）、FileEditor（文档库读写与项目运行）。
//
// 硬边界：全仓只有 protocol/agentbase 能发起推理，请求组装、流式解析、tool_calls
// 分片聚合、重试、超时、推理实况与失败上报都在那里。chain/、engine/、server/、
// executors/ 不得自行拼模型请求、解析流式输出或持有 StreamRequest；链只做编排，
// 需要模型能力时调用本目录的智能体，引擎侧只能声明函数钩子由装配层注入。
//
// 本文件不再维护逐条目录清单，权威来源是：
//   - 智能体分层与职责：docs/code-wiki/09-调度与智能体分层.md
//   - 智能体 ID 与显示名：protocol/agentinfo
//   - 边界自查命令与新增智能体清单：.trae/rules/agent-chain-boundary.md
//
// 跨多智能体的链路编排不在本目录，而在 chain/ 的七条链：thinking（思维链）、
// search（搜索链）、life（生活链）、viewer（观影链）、storyboard（分镜链）、
// forum（论坛链）、desktop（桌控链，包名 desktopchain）。页面注入型智能体脚本
// （Mini-LTP / Self-LTP / SharedInput）由 server/ltpxhost 经 go:embed 内嵌在
// scripts/*.js 中单独下发，源码与下发方同包，随二进制一起发布。
package agents
