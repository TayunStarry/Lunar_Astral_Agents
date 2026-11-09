package WishManager

// dump.go 愿望管理的上下文导出：最近一次指令与操作结果、当前愿望清单与心情。
//
// 愿望管理按次执行、不保留跨轮对话；导出覆盖本轮在途状态与心愿链路快照。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// lastRunRecord 最近一次愿望管理执行记录。
type lastRunRecord struct {
	// Instruction 本轮自然语言指令
	Instruction string `json:"instruction"`
	// OK 本轮是否成功
	OK bool `json:"ok"`
	// Action 本轮实际操作
	Action string `json:"action,omitempty"`
	// Item 本轮新增 / 改写 / 完成 / 删除的愿望
	Item *wishchain.Wish `json:"item,omitempty"`
	// NeedConfirm 本轮是否因容量上限需要确认
	NeedConfirm bool `json:"need_confirm,omitempty"`
	// Summary 本轮操作简述
	Summary string `json:"summary,omitempty"`
	// Error 本轮错误（成功为空）
	Error string `json:"error,omitempty"`
	// At 本轮记录时间
	At time.Time `json:"at"`
}

var (
	// lastRunMu 保护最近一次执行记录
	lastRunMu sync.Mutex
	// lastRun 最近一次执行记录（nil 表示本进程内尚未执行）
	lastRun *lastRunRecord
)

// exportContext 愿望管理的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastRunMu.Lock()
	record := lastRun
	lastRunMu.Unlock()
	out := map[string]any{
		"kind":         "wish-chain-context",
		"generated_at": time.Now().Format(time.RFC3339),
		"wishes":       wishchain.All(),
		"wish_summary": wishchain.FormatList(),
		"mood":         wishchain.SnapshotMood().Label,
		"links":        wishchain.RecentLinks(20),
	}
	if record == nil {
		out["note"] = "本次进程内还没有执行过愿望管理"
		return out, nil
	}
	out["last_run"] = record
	return out, nil
}

// recordRun 记录本轮执行结果，供导出与修复读取。
func recordRun(instruction string, result Result, err error) {
	record := &lastRunRecord{
		Instruction: instruction,
		OK:          result.OK && err == nil,
		Action:      result.Action,
		Item:        result.Item,
		NeedConfirm: result.NeedConfirm,
		Summary:     result.Summary,
		At:          time.Now(),
	}
	if err != nil {
		record.Error = err.Error()
	}
	lastRunMu.Lock()
	lastRun = record
	lastRunMu.Unlock()
}

// resetRun 清空最近一次执行记录。
func resetRun() string {
	lastRunMu.Lock()
	had := lastRun != nil
	lastRun = nil
	lastRunMu.Unlock()
	if had {
		return "已清空愿望管理的最近一次执行记录"
	}
	return "愿望管理当前没有执行记录"
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          AgentID,
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: "愿望管理最近一次指令、操作结果与错误，以及当前愿望清单与心情",
		Export:      exportContext,
	})
}
