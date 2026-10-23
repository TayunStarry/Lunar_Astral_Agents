package console

import (
	"bytes"
	"encoding/json"
	"time"
)

// ==== 核心智能实况的导出（轮次记录 transcript） ====
//
// 实况屏的数据是**独一份**的运行时资料，任何日志通道都不覆盖它：
//   - 原始日志：只有启动/工具/模型调用的一行行文本，没有任何"轮次"结构；
//   - 月华日志：对话链运行输出，与模型调用轮次没有一一对应关系；
//   - 智能体上下文：给模型看的 messages（`/debug/dump/context`），是**输入**，
//     不含模型的思考流、也不含每轮的工具调用顺序与耗时。
//
// 因此这里导出的是「模型调用轮次」：每轮的角色、开始/结束时刻、思考全文、输出全文、
// 工具调用序列，外加进程累计的模型调用计量。
//
// 两点必须如实标注（否则会把"环形缓冲的一段"误当成"全部历史"）：
//   - 轮次数量有上限（turnHistoryCap），超出即淘汰，故写入 created/dropped 计数；
//   - 单轮思考/输出文本有字符上限（turnTextCap），超限只保留**尾部**，
//     故写入 text_cap 与该轮是否发生截断的标记。
//
// 另注：实况屏渲染时思考/输出只显示末尾若干行摘要（8/14 行），
// 本导出给出的是**未被界面截断的完整文本**。

// TranscriptTurn 一轮模型调用的导出结构
type TranscriptTurn struct {
	Role        string   `json:"role"`
	Active      bool     `json:"active"`      // 进行中（未收到结束事件）
	StartedAt   string   `json:"started_at"`  // 本地时间（毫秒），如 2026-10-16 21:20:31.482
	FinishedAt  string   `json:"finished_at"` // 进行中时为空
	DurationMS  int64    `json:"duration_ms"`
	Thinking    string   `json:"thinking"`
	Content     string   `json:"content"`
	Tools       []string `json:"tools"`
	ToolCount   int      `json:"tool_count"`
	ThinkingLen int      `json:"thinking_len"` // 字符数（含首尾空白）
	ContentLen  int      `json:"content_len"`
	// Truncated 真表示该轮思考或输出达到 turnTextCap、只保留了尾部
	Truncated bool `json:"truncated"`
}

// TranscriptMeta 导出的元信息：说明这份数据的完整性与来源
type TranscriptMeta struct {
	Kind        string         `json:"kind"`
	GeneratedAt string         `json:"generated_at"`
	Source      string         `json:"source"`
	Note        string         `json:"note"`
	HistoryCap  int            `json:"history_cap"`
	HistoryLen  int            `json:"history_len"`
	Created     int            `json:"created_total"`
	Dropped     int            `json:"dropped"`
	TextCap     int            `json:"text_cap"`
	TurnCount   int            `json:"turn_count"`
	HasActive   bool           `json:"has_active_turn"`
	ModelCalls  ModelCallStats `json:"model_calls"`
}

// TranscriptDump 实况导出的完整载荷
type TranscriptDump struct {
	Meta  TranscriptMeta   `json:"meta"`
	Turns []TranscriptTurn `json:"turns"`
}

// BuildTranscript 汇总当前实况轮次为可序列化结构。
//
// 在**同一把锁内**完成快照与元信息读取：否则可能与并发的轮次归档交错，
// 出现"元信息说 12 轮、实际拿到 11 轮"这类自相矛盾的导出。
func BuildTranscript() TranscriptDump {
	streamMu.Lock()
	turns := snapshotTurnsLocked()
	info := turnStoreInfoLocked()
	streamMu.Unlock()

	// 计量器自带锁，放在 streamMu 之外取，避免两条锁顺序耦合
	stats := GetModelCallStats()

	out := make([]TranscriptTurn, 0, len(turns))
	for _, t := range turns {
		if t == nil {
			continue
		}
		item := TranscriptTurn{
			Role:        t.Role,
			Active:      t.Active,
			StartedAt:   t.StartedAt.Format("2006-01-02 15:04:05.000"),
			Thinking:    t.Thinking,
			Content:     t.Content,
			Tools:       t.Tools,
			ToolCount:   len(t.Tools),
			ThinkingLen: len(t.Thinking),
			ContentLen:  len(t.Content),
		}
		if !t.FinishedAt.IsZero() {
			item.FinishedAt = t.FinishedAt.Format("2006-01-02 15:04:05.000")
			item.DurationMS = t.FinishedAt.Sub(t.StartedAt).Milliseconds()
		}
		if len(item.Tools) == 0 {
			item.Tools = []string{} // 显式空数组，便于 jq 直接 .tools[]
		}
		// 文本达到上限即认为发生过尾部截断（appendTurnText 只保留尾部）
		item.Truncated = len(t.Thinking) >= info.TextCap || len(t.Content) >= info.TextCap
		out = append(out, item)
	}

	meta := TranscriptMeta{
		Kind:        "lunar-agent-transcript",
		GeneratedAt: time.Now().Format(time.RFC3339),
		Source:      "TUI 核心智能实况（模型调用轮次缓冲）",
		Note: "思考/输出为未按界面摘要截断的全文；轮次与单轮文本均受环形上限约束，" +
			"created_total 与 dropped 反映累计与淘汰量，truncated 标记该轮是否只保留了尾部。",
		HistoryCap: info.HistoryCap,
		HistoryLen: info.HistoryLen,
		Created:    info.Created,
		Dropped:    info.Dropped,
		TextCap:    info.TextCap,
		TurnCount:  len(out),
		HasActive:  info.HasActive,
		ModelCalls: stats,
	}
	return TranscriptDump{Meta: meta, Turns: out}
}

// BuildTranscriptJSON 把实况导出序列化为 JSON 文本（供导出端点落盘）。
//
// 关闭 HTML 转义：模型输出里 `->`、`&&`、`<tag>` 之类极常见，转义后整份文件
// 几乎无法用肉眼扫读（与原始日志导出同一取舍）。
func BuildTranscriptJSON() (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(BuildTranscript()); err != nil {
		return "", err
	}
	return buf.String(), nil
}
