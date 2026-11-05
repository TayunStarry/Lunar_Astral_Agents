package SearchPlanner

// dump.go 搜索者-规划的导出：最近一次结构化决策与原始指令。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

type lastPlanRecord struct {
	Instruction string    `json:"instruction"`
	Decision    Decision  `json:"decision"`
	At          time.Time `json:"at"`
}

var (
	lastPlanMu   sync.Mutex
	lastPlanItem *lastPlanRecord
)

// exportContext 搜索者-规划的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastPlanMu.Lock()
	record := lastPlanItem
	lastPlanMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "search-planner-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有执行过搜索规划",
		}, nil
	}
	return map[string]any{
		"kind":          "search-planner-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 1,
		"last_plan":     record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       "搜索者-规划",
		Group:       "Go 智能体",
		Description: "搜索者-规划最近一次结构化决策（管线 / 检索词 / 参数 / 把握）",
		Export:      exportContext,
	})
}

func recordLastPlan(instruction string, decision Decision) {
	lastPlanMu.Lock()
	defer lastPlanMu.Unlock()
	lastPlanItem = &lastPlanRecord{
		Instruction: instruction,
		Decision:    decision,
		At:          time.Now(),
	}
}
