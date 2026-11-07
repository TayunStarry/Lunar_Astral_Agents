package Musician

// dump.go 演奏者的上下文导出：最近一次演奏的指令、曲目与错误。
//
// 演奏按次执行、不保留跨轮对话；导出只覆盖本轮在途状态，dump 导出与智能修复
// 的 Inspect 共用这一份。

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// agentID 演奏者的导出与修复标识。
const agentID = "musician"

// trackRecord 本轮产出的曲目（与 Result 的字段一一对应）。
type trackRecord struct {
	// Title 作品标题
	Title string `json:"title"`
	// Description 推送卡片上的一行描述
	Description string `json:"description,omitempty"`
	// Duration 音频时长（秒）
	Duration float64 `json:"duration"`
	// AudioURL 前端可直接播放的音频地址
	AudioURL string `json:"audio_url,omitempty"`
	// Summary 回传给对话者的作品介绍
	Summary string `json:"summary,omitempty"`
}

// lastRunRecord 最近一次演奏执行记录。
type lastRunRecord struct {
	// Instruction 本轮创作描述
	Instruction string `json:"instruction"`
	// OK 本轮是否成功
	OK bool `json:"ok"`
	// Track 本轮产出的曲目（失败为空）
	Track *trackRecord `json:"track,omitempty"`
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

// exportContext 演奏者的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	_ = includeImages
	lastRunMu.Lock()
	record := lastRun
	lastRunMu.Unlock()
	out := map[string]any{
		"kind":         "musician-context",
		"generated_at": time.Now().Format(time.RFC3339),
	}
	if record == nil {
		out["note"] = "本次进程内还没有执行过演奏"
		return out, nil
	}
	out["last_run"] = record
	return out, nil
}

// recordRun 记录本轮演奏结果，供导出与修复读取。
func recordRun(instruction string, result Result, err error) {
	record := &lastRunRecord{
		Instruction: instruction,
		OK:          err == nil,
		At:          time.Now(),
	}
	if err != nil {
		record.Error = err.Error()
	} else {
		record.Track = &trackRecord{
			Title:       result.Title,
			Description: result.Description,
			Duration:    result.Duration,
			AudioURL:    result.AudioURL,
			Summary:     result.Summary,
		}
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
		return "已清空" + roleLabel + "的最近一次演奏记录"
	}
	return roleLabel + "当前没有演奏记录"
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          agentID,
		Label:       roleLabel,
		Group:       "Go 智能体",
		Description: roleLabel + "最近一次创作描述、曲目与错误",
		Export:      exportContext,
	})
}
