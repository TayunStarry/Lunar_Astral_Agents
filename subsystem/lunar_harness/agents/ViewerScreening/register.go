package ViewerScreening

// register.go 观影者-阅片的登记：工具目录、上下文导出与智能修复。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// lastScreeningRecord 最近一次阅片结果。
type lastScreeningRecord struct {
	Summary string    `json:"summary"`
	Topics  []string  `json:"topics,omitempty"`
	Tone    string    `json:"tone,omitempty"`
	At      time.Time `json:"at"`
}

var (
	lastScreeningMu   sync.Mutex
	lastScreeningItem *lastScreeningRecord
)

// exportContext 观影者-阅片的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastScreeningMu.Lock()
	record := lastScreeningItem
	lastScreeningMu.Unlock()
	if record == nil {
		return map[string]any{
			"kind":          "viewer-screening-context",
			"generated_at":  time.Now().Format(time.RFC3339),
			"message_count": 0,
			"note":          "本次进程内还没有执行过阅片",
		}, nil
	}
	return map[string]any{
		"kind":            "viewer-screening-context",
		"generated_at":    time.Now().Format(time.RFC3339),
		"message_count":   1,
		"last_impression": record,
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       Role,
		Group:       "Go 智能体",
		Description: "观影者-阅片最近一次视频印象（摘要 / 主题 / 情绪）",
		Export:      exportContext,
	})
	agentrepair.RegisterResettable(AgentID, Role,
		"观影者-阅片的最近一次视频印象；reset 清空该记录",
		resetScreeningState,
		func(int) (any, error) { return exportContext(false) })
	toolreg.Register(AgentID,
		toolreg.Schema{Name: "pick_keywords", Description: "综合指令、偏好、心情与记忆决定本轮选片关键词。"},
		toolreg.Schema{Name: "screen_video", Description: "依据标题 / 标签 / 简介 / 评论 / 弹幕生成结构化视频印象。"},
		toolreg.Schema{Name: "watch_slice", Description: "理解一个 60 秒切片（抽帧后的图文混排）。"},
		toolreg.Schema{Name: "watch_merge", Description: "把切片理解合成为第一人称观后感受。"},
	)
}

// ToolCatalog 返回阅片的阶段能力（阅片不使用 function calling）。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: AgentID, Name: Role, Source: "agents/ViewerScreening",
		Note:  "观影者之一：只看画面与公开信息，产出视频印象、切片理解与观后感受。",
		Tools: agentinfo.ToolsFromRegistry(AgentID),
	}}
}

// recordImpression 记录最近一次阅片结果。
func recordImpression(impression Impression) {
	lastScreeningMu.Lock()
	defer lastScreeningMu.Unlock()
	lastScreeningItem = &lastScreeningRecord{
		Summary: impression.Summary,
		Topics:  impression.Topics,
		Tone:    impression.Tone,
		At:      time.Now(),
	}
}

// resetScreeningState 清空最近一次阅片记录。
func resetScreeningState() string {
	lastScreeningMu.Lock()
	had := lastScreeningItem != nil
	lastScreeningItem = nil
	lastScreeningMu.Unlock()
	if had {
		logger.Info("CrystalAstral", "观影者-阅片最近印象记录已清空")
		return "已清空观影者-阅片的最近印象记录"
	}
	return "观影者-阅片当前没有印象记录"
}
