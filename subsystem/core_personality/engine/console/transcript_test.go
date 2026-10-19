package console

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// resetTurnStore 清空轮次缓冲与计数（各子测试互不干扰；测试间不并发）
func resetTurnStore() {
	streamMu.Lock()
	currentTurn = nil
	turnHistory = nil
	turnCreated = 0
	turnDropped = 0
	streamMu.Unlock()
}

// 覆盖：实况导出的基本结构——每轮的角色 / 思考 / 输出 / 工具序列 / 时长都要在。
//
// 这是「把核心智能实况加入可导出数据」的核心契约。实况轮次是独一份的运行时资料：
// 原始日志没有"轮次"结构，`/debug/dump/context` 只有给模型看的 messages（输入）、
// 不含模型思考流与工具调用顺序/耗时。
func TestBuildTranscriptCapturesTurns(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindThink, "先看图")
	StreamDelta("对话者", KindThink, "，再回答")
	StreamDelta("对话者", KindContent, "这是回答")
	StreamTool("对话者", "list_images", `{"limit":5}`)
	StreamToolResult("对话者", "共 2 张")
	StreamEnd("对话者")

	dump := BuildTranscript()
	if dump.Meta.Kind != "lunar-agent-transcript" {
		t.Fatalf("kind 标记错误: %q", dump.Meta.Kind)
	}
	if dump.Meta.TurnCount != 1 || len(dump.Turns) != 1 {
		t.Fatalf("轮次数错误: meta=%d turns=%d", dump.Meta.TurnCount, len(dump.Turns))
	}
	turn := dump.Turns[0]
	if turn.Role != "对话者" {
		t.Fatalf("角色错误: %q", turn.Role)
	}
	if turn.Active {
		t.Fatal("已结束的轮次不应标记为进行中")
	}
	if turn.Thinking != "先看图，再回答" {
		t.Fatalf("思考内容错误: %q", turn.Thinking)
	}
	if turn.Content != "这是回答" {
		t.Fatalf("输出内容错误: %q", turn.Content)
	}
	if turn.ToolCount != 1 || len(turn.Tools) != 1 || !strings.HasPrefix(turn.Tools[0], "list_images") {
		t.Fatalf("工具调用记录错误: %#v", turn.Tools)
	}
	if turn.StartedAt == "" || turn.FinishedAt == "" {
		t.Fatalf("时刻缺失: started=%q finished=%q", turn.StartedAt, turn.FinishedAt)
	}
	if turn.DurationMS < 0 {
		t.Fatalf("时长异常: %d", turn.DurationMS)
	}
	if turn.Truncated {
		t.Fatal("短文本不应标记为截断")
	}

	// 进行中的轮次要能被导出，且标记为 active（不丢"正在跑"这个事实）
	StreamBegin("绘制者")
	StreamDelta("绘制者", KindContent, "画到一半")
	live := BuildTranscript()
	if len(live.Turns) != 2 {
		t.Fatalf("进行中轮次未纳入导出: %d", len(live.Turns))
	}
	if !live.Turns[1].Active {
		t.Fatal("进行中轮次应标记 active")
	}
	if !live.Meta.HasActive {
		t.Fatal("meta.has_active_turn 应为真")
	}
	if live.Turns[1].FinishedAt != "" {
		t.Fatalf("进行中轮次不应有结束时刻: %q", live.Turns[1].FinishedAt)
	}
}

// 覆盖：环形上限的**如实告知**——淘汰发生时要能在元信息里看出来。
//
// 若不说清"只留最近 N 轮"，使用者会把一份残缺导出误当作全部历史。
func TestBuildTranscriptReportsEviction(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	// 造出超过上限的轮次，触发淘汰
	total := turnHistoryCap + 5
	for i := 0; i < total; i++ {
		StreamBegin("对话者")
		StreamDelta("对话者", KindContent, "轮次内容")
		StreamEnd("对话者")
	}

	dump := BuildTranscript()
	if dump.Meta.Created != total {
		t.Fatalf("累计轮数错误: 期望 %d，实际 %d", total, dump.Meta.Created)
	}
	if dump.Meta.Dropped != 5 {
		t.Fatalf("淘汰计数错误: 期望 5，实际 %d", dump.Meta.Dropped)
	}
	if dump.Meta.HistoryCap != turnHistoryCap {
		t.Fatalf("上限未如实上报: %d", dump.Meta.HistoryCap)
	}
	if len(dump.Turns) != turnHistoryCap {
		t.Fatalf("保留轮数错误: 期望 %d，实际 %d", turnHistoryCap, len(dump.Turns))
	}
	// 淘汰量与保留量必须自洽
	if dump.Meta.Created != dump.Meta.Dropped+dump.Meta.HistoryLen {
		t.Fatalf("计数不自洽: created=%d dropped=%d history=%d",
			dump.Meta.Created, dump.Meta.Dropped, dump.Meta.HistoryLen)
	}
}

// 覆盖：单轮文本达到上限时标记 truncated（appendTurnText 只保留尾部）。
func TestBuildTranscriptFlagsTruncatedTurn(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	// 恰好在写入时触发尾部截断（appendTurnText 以 turnTextCap 为界）
	StreamDelta("对话者", KindContent, strings.Repeat("x", turnTextCap+100))
	StreamEnd("对话者")

	dump := BuildTranscript()
	if len(dump.Turns) != 1 {
		t.Fatalf("轮次数错误: %d", len(dump.Turns))
	}
	if !dump.Turns[0].Truncated {
		t.Fatalf("超长文本应标记 truncated（长度 %d，上限 %d）",
			dump.Turns[0].ContentLen, dump.Meta.TextCap)
	}
	if dump.Turns[0].ContentLen > turnTextCap {
		t.Fatalf("文本未被截断: %d > %d", dump.Turns[0].ContentLen, turnTextCap)
	}
}

// 覆盖：JSON 序列化可用——合法 JSON、含元信息与轮次、且 `->` 等不被 HTML 转义。
//
// 模型输出里 `->`、`&&`、`<tag>` 极常见，转义后整份文件无法肉眼扫读
// （与原始日志导出同一取舍）。
func TestBuildTranscriptJSONIsReadable(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	StreamBegin("对话者")
	StreamDelta("对话者", KindContent, "a -> b && <div>")
	StreamEnd("对话者")

	text, err := BuildTranscriptJSON()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	for _, bad := range []string{`\u003e`, `\u003c`, `\u0026`} {
		if strings.Contains(text, bad) {
			t.Fatalf("含 HTML 转义 %s:\n%s", bad, text)
		}
	}
	if !strings.Contains(text, "a -> b && <div>") {
		t.Fatalf("正文未原样保留:\n%s", text)
	}

	var parsed TranscriptDump
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("导出内容不是合法 JSON: %v", err)
	}
	if parsed.Meta.TurnCount != 1 {
		t.Fatalf("解析后轮次数错误: %d", parsed.Meta.TurnCount)
	}
	if parsed.Turns[0].Content != "a -> b && <div>" {
		t.Fatalf("解析后正文错误: %q", parsed.Turns[0].Content)
	}
}

// 覆盖：无数据时也要产出结构完整的空导出（而不是报错或产出 null）。
//
// 用户可能在没有对话时按 t，此时应得到"0 轮"的合法文件，
// 打开就能确认「不是导出失败，而是确实还没有轮次」。
func TestBuildTranscriptEmptyIsWellFormed(t *testing.T) {
	resetTurnStore()
	defer resetTurnStore()

	text, err := BuildTranscriptJSON()
	if err != nil {
		t.Fatalf("空导出失败: %v", err)
	}
	var parsed TranscriptDump
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("空导出不是合法 JSON: %v", err)
	}
	if parsed.Meta.TurnCount != 0 {
		t.Fatalf("空导出轮次数应为 0: %d", parsed.Meta.TurnCount)
	}
	if parsed.Turns == nil {
		t.Fatal("turns 应为空数组而非 null（便于 jq 直接取 .turns[]）")
	}
	if parsed.Meta.GeneratedAt == "" {
		t.Fatal("缺少生成时刻")
	}
	// generated_at 应是可解析的 RFC3339
	if _, err := time.Parse(time.RFC3339, parsed.Meta.GeneratedAt); err != nil {
		t.Fatalf("generated_at 不是 RFC3339: %q", parsed.Meta.GeneratedAt)
	}
}
