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

// lastReviewRecord 最近一次审核结论。
type lastReviewRecord struct {
	Title  string    `json:"title"`
	Pass   bool      `json:"pass"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
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
		Description: Role + "最近一次合规复核的结论",
		Export:      exportContext,
	})
	agentrepair.RegisterReadonly(AgentID, Role,
		Role+"的最近一次审核结论（只读）",
		func(int) (any, error) { return exportContext(false) })
	toolreg.Register(AgentID, toolreg.Schema{
		Name:        "review_video",
		Description: "结合文字材料与封面复核合规性，输出 pass / reason。",
	})
}

// ToolCatalog 返回审查的阶段能力。
// submit_review 只是模型与链之间的提交约定，没有执行器，因此不登记到工具目录。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: Role,
		Note:  "观影者之一：只做合规与风险判定，并附带一句观影评价。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}

// recordReview 记录最近一次审核结论。
func recordReview(title string, review Verdict) {
	lastReviewMu.Lock()
	defer lastReviewMu.Unlock()
	lastReviewItem = &lastReviewRecord{Title: title, Pass: review.Pass, Reason: review.Reason, At: time.Now()}
}

// resetReviewState 清空最近一次审核记录。
func resetReviewState() string {
	lastReviewMu.Lock()
	had := lastReviewItem != nil
	lastReviewItem = nil
	lastReviewMu.Unlock()
	if had {
		logger.Info("CrystalAstral", Role+"最近审核记录已清空")
		return "已清空" + Role + "的最近审核记录"
	}
	return Role + "当前没有审核记录"
}
