package PlanManager

// dump.go 计划管理的上下文导出：最近一次指令与操作结果、当前计划表。
//
// 计划管理按次执行、不保留跨轮对话；导出只覆盖本轮在途状态（最近操作与错误）
// 与计划表现状，dump 导出与智能修复的 Inspect 共用这一份。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/plans"
	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// agentID 计划管理的导出与修复标识。
const agentID = "plan_manager"

// lastRunRecord 最近一次计划管理执行记录。
type lastRunRecord struct {
	// Instruction 本轮自然语言指令
	Instruction string `json:"instruction"`
	// OK 本轮是否成功
	OK bool `json:"ok"`
	// Action 本轮实际操作（query / create / update / delete）
	Action string `json:"action,omitempty"`
	// Item 本轮新增 / 改写 / 删除的计划项
	Item *plans.Item `json:"item,omitempty"`
	// Summary 本轮操作简述
	Summary string `json:"summary,omitempty"`
	// Error 本轮错误（成功为空）
	Error string `json:"error,omitempty"`
	// At 本轮记录时间
	At time.Time `json:"at"`
}

var (
	// lastRunMu 保护最近一次执行记录。
	lastRunMu sync.Mutex
	// lastRun 最近一次执行记录（nil 表示本进程内尚未执行）。
	lastRun *lastRunRecord
)

// exportContext 计划管理的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastRunMu.Lock()
	record := lastRun
	lastRunMu.Unlock()
	out := map[string]any{
		"kind":         "plan-manager-context",
		"generated_at": time.Now().Format(time.RFC3339),
		"plans":        plans.List(),
	}
	if record == nil {
		out["note"] = "本次进程内还没有执行过计划管理"
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
		return "已清空计划管理的最近一次执行记录"
	}
	return "计划管理当前没有执行记录"
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          agentID,
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: "计划管理最近一次指令、操作结果与错误，以及当前计划表",
		Export:      exportContext,
	})
}
