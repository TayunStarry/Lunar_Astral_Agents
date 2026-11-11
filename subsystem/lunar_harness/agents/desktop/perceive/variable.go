//go:build windows

package perceive

// variable.go 「桌面感知」成员的身份、提示词与只读工具定义。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// AgentID 成员唯一标识：推理实况、故障修复、数据导出与工具目录共用这一个 ID。
const AgentID = "desktop_perceive"

// modelRole 实况显示名（四字，取自 protocol/agentinfo 统一约定）。
const modelRole = agentinfo.NameDesktopPerceive

// autoLoopMaxRetries 模型输出命中重复后，单次模型调用的额外重试次数。
const autoLoopMaxRetries = 2

// modelTimeout 单次模型调用的最长等待时间。
const modelTimeout = 120 * time.Second

// maxRounds 单次模型调用的最大工具轮次（本成员无工具，一次调用即结束）。
const maxRounds = 1

// promptUIAReader 提示词：把 UI 树整理成与任务相关的元素清单。
var promptUIAReader = desktop.MustPrompt("desktopUIAReader.md")
