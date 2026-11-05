package ViewerCensor

// register.go 观影者-审查的登记：工具目录、上下文导出与智能修复。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// lastReviewRecord 最近一次审核结果（原始文本）。
type lastReviewRecord struct {
	Title string    `json:"title"`
	Text  string    `json:"text"`
	At    time.Time `json:"at"`
}

var (
	lastReviewMu   sync.Mutex
	lastReviewItem *lastReviewRecord
)

// exportContext 观影者-审查的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastReviewMu.Lock()
	record := lastReviewItem
	lastReviewMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "viewer-censor-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有执行过合规审查",
		}, nil
	}
	return map[string]any{
		"kind":          "viewer-censor-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 1,
		"last_review":   record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       Role,
		Group:       "Go 智能体",
		Description: "观影者-审查最近一次合规复核的模型原始输出",
		Export:      exportContext,
	})
	agentrepair.RegisterResettable(AgentID, Role,
		"观影者-审查的最近一次审核输出；reset 清空该记录",
		resetReviewState,
		func(int) (any, error) { return exportContext(false) })
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "review_video",
		Description: "结合文字材料与封面复核合规性，输出 pass / reason / evaluation / slices。",
	})
}

// ToolCatalog 返回审查的阶段能力（审查不使用 function calling）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: Role, Source: "agents/ViewerCensor",
		Note:  "观影者之一：只做合规与风险判定，并附带一句观影评价。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}

// recordReview 记录最近一次审核输出。
func recordReview(title, text string) {
	lastReviewMu.Lock()
	defer lastReviewMu.Unlock()
	lastReviewItem = &lastReviewRecord{Title: title, Text: text, At: time.Now()}
}

// resetReviewState 清空最近一次审核记录。
func resetReviewState() string {
	lastReviewMu.Lock()
	had := lastReviewItem != nil
	lastReviewItem = nil
	lastReviewMu.Unlock()
	if had {
		logger.Info("CrystalAstral", "观影者-审查最近审核记录已清空")
		return "已清空观影者-审查的最近审核记录"
	}
	return "观影者-审查当前没有审核记录"
}
