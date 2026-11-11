package ViewerShare

// register.go 观影者-分享的登记：工具目录、上下文导出与智能修复。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// lastShareRecord 最近一次对外文案。
type lastShareRecord struct {
	Kind string    `json:"kind"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

var (
	lastShareMu   sync.Mutex
	lastShareItem *lastShareRecord
)

// exportContext 观影者-分享的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastShareMu.Lock()
	record := lastShareItem
	lastShareMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "viewer-share-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有生成过分享文案",
		}, nil
	}
	return map[string]any{
		"kind":          "viewer-share-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 1,
		"last_share":    record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       Role,
		Group:       "Go 智能体",
		Description: Role + "最近一次对外文案（观影动态 / 分享评论）",
		Export:      exportContext,
	})
	agentrepair.RegisterReadonly(AgentID, Role,
		Role+"的最近一次对外文案（只读）",
		func(int) (any, error) { return exportContext(false) })
	toolreg.Register(AgentID,
		toolreg.Schema{Name: "write_dynamic", Description: "生成一轮观影动态正文。"},
		toolreg.Schema{Name: "write_share", Description: "生成一条 @分享评论。"},
	)
}

// ToolCatalog 返回分享的阶段能力（分享不使用 function calling）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: Role,
		Note:  "观影者之一：只生成对外分享文案（观影动态与 @分享评论）。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}

// recordShare 记录最近一次对外文案。
func recordShare(kind, text string) {
	lastShareMu.Lock()
	defer lastShareMu.Unlock()
	lastShareItem = &lastShareRecord{Kind: kind, Text: text, At: time.Now()}
}

// resetShareState 清空最近一次分享记录。
func resetShareState() string {
	lastShareMu.Lock()
	had := lastShareItem != nil
	lastShareItem = nil
	lastShareMu.Unlock()
	if had {
		logger.Info("CrystalAstral", Role+"最近文案记录已清空")
		return "已清空" + Role + "的最近文案记录"
	}
	return Role + "当前没有文案记录"
}
