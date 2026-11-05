package ViewerCritic

// register.go 观影者-评论的登记：工具目录、上下文导出与智能修复。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// lastCriticRecord 最近一次点评输出。
type lastCriticRecord struct {
	Kind string    `json:"kind"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

var (
	lastCriticMu   sync.Mutex
	lastCriticItem *lastCriticRecord
)

// exportContext 观影者-评论的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastCriticMu.Lock()
	record := lastCriticItem
	lastCriticMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "viewer-critic-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有生成过点评",
		}, nil
	}
	return map[string]any{
		"kind":          "viewer-critic-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 1,
		"last_comment":  record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       Role,
		Group:       "Go 智能体",
		Description: "观影者-评论最近一次点评输出（互动回应 / 观后回顾）",
		Export:      exportContext,
	})
	agentrepair.RegisterResettable(AgentID, Role,
		"观影者-评论的最近一次点评输出；reset 清空该记录",
		resetCriticState,
		func(int) (any, error) { return exportContext(false) })
	toolreg.Register(AgentID,
		toolreg.Schema{Name: "reply_inbox", Description: "按场景生成一句自然回应（注入人设与长期记忆）。"},
		toolreg.Schema{Name: "round_memory", Description: "把本轮观影体验写成人格化记忆。"},
	)
}

// ToolCatalog 返回评论的阶段能力（评论不使用 function calling）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: Role, Source: "agents/ViewerCritic",
		Note:  "观影者之一：只生成点评类文本（互动回应与观后回顾）。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}

// recordCritic 记录最近一次点评输出。
func recordCritic(kind, text string) {
	lastCriticMu.Lock()
	defer lastCriticMu.Unlock()
	lastCriticItem = &lastCriticRecord{Kind: kind, Text: text, At: time.Now()}
}

// resetCriticState 清空最近一次点评记录。
func resetCriticState() string {
	lastCriticMu.Lock()
	had := lastCriticItem != nil
	lastCriticItem = nil
	lastCriticMu.Unlock()
	if had {
		logger.Info("CrystalAstral", "观影者-评论最近点评记录已清空")
		return "已清空观影者-评论的最近点评记录"
	}
	return "观影者-评论当前没有点评记录"
}
