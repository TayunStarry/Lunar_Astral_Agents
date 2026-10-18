package TuiSystem

import (
	"strings"
	"time"
)

// ==== 智能体流式事件 API（由 core_personality 的 syncFetch 流式桥调用） ====

// StreamBegin 标记一次模型调用的开始（角色标签由请求头 X-Lunar-Role 提供）
func StreamBegin(role string) {
	streamMu.Lock()
	defer streamMu.Unlock()

	// 上一轮未正常收尾则先落历史
	if currentTurn != nil && currentTurn.Active {
		currentTurn.Active = false
		currentTurn.FinishedAt = time.Now()
		archiveTurnLocked()
	}
	currentTurn = &AgentTurn{Role: role, Active: true, StartedAt: time.Now()}
	emitStream(StreamEvent{Role: role, Kind: KindBegin})
}

// StreamDelta 追加一段流式增量（思考/输出）
func StreamDelta(role string, kind StreamKind, text string) {
	if text == "" {
		return
	}
	streamMu.Lock()
	defer streamMu.Unlock()

	turn := attachTurnLocked(role)
	if turn != nil {
		switch kind {
		case KindThink:
			turn.Thinking = appendTurnText(turn.Thinking, text)
		case KindContent:
			turn.Content = appendTurnText(turn.Content, text)
		}
	}
	emitStream(StreamEvent{Role: role, Kind: kind, Text: text})
}

// StreamTool 记录一次工具调用（聚合完成事件：名称 + 参数摘要）
func StreamTool(role, name, args string) {
	streamMu.Lock()
	defer streamMu.Unlock()

	entry := name
	if trimmed := summarizeArgs(args); trimmed != "" {
		entry = name + " " + trimmed
	}
	if turn := attachTurnLocked(role); turn != nil {
		turn.Tools = append(turn.Tools, entry)
	}
	emitStream(StreamEvent{Role: role, Kind: KindTool, Text: entry})
}

// attachTurnLocked 把增量/工具挂到合适的轮次上（需持有 streamMu）：
// 有进行中的轮次就挂它；否则挂到最近归档的一轮；都没有时返回 nil（只发事件，不造轮）。
//
// 为什么不在 currentTurn 为空时新建轮次：历史上这样造出来的「幽灵轮次」没有结束事件，
// 实况屏会把它当成永远进行中的一轮（界面上就是一条工具调用挂着"进行中"不下来），
// 而当轮的思考/正文却留在了上一轮里，导出（transcript）也会多出若干只有工具的空轮次。
func attachTurnLocked(_ string) *AgentTurn {
	if currentTurn != nil {
		return currentTurn
	}
	if n := len(turnHistory); n > 0 {
		return turnHistory[n-1]
	}
	return nil
}

// StreamToolResult 记录工具返回摘要
func StreamToolResult(role, summary string) {
	emitStream(StreamEvent{Role: role, Kind: KindToolResult, Text: summary})
}

// StreamEnd 标记模型调用结束（落历史轮次）
func StreamEnd(role string) {
	streamMu.Lock()
	defer streamMu.Unlock()
	if currentTurn != nil {
		currentTurn.Active = false
		currentTurn.FinishedAt = time.Now()
		archiveTurnLocked()
		currentTurn = nil
	}
	emitStream(StreamEvent{Role: role, Kind: KindEnd})
}

// archiveTurnLocked 将已结束的当前轮次归档（需持有 streamMu）。
// 同时记账「累计创建轮数」与「因环形上限被淘汰的轮数」：实况屏是环形缓冲，
// 数据导出必须能如实告知"这次导出拿到的是全部还是仅最近一段"。
func archiveTurnLocked() {
	if currentTurn == nil {
		return
	}
	turnCreated++
	turnHistory = append(turnHistory, currentTurn)
	if len(turnHistory) > turnHistoryCap {
		dropped := len(turnHistory) - turnHistoryCap
		turnDropped += dropped
		turnHistory = turnHistory[len(turnHistory)-turnHistoryCap:]
	}
}

// TurnStoreInfo 实况轮次缓冲的容量与淘汰情况（数据导出写进元信息用）
type TurnStoreInfo struct {
	HistoryCap int // 历史轮次保留上限
	HistoryLen int // 当前保留的历史轮次条数（不含进行中轮次）
	Created    int // 进程启动以来累计完成（归档）的轮数
	Dropped    int // 因上限被淘汰的轮数
	TextCap    int // 单轮思考/输出文本上限（字符）
	HasActive  bool
}

// turnStoreInfoLocked 读取轮次缓冲信息（需持有 streamMu）
func turnStoreInfoLocked() TurnStoreInfo {
	return TurnStoreInfo{
		HistoryCap: turnHistoryCap,
		HistoryLen: len(turnHistory),
		Created:    turnCreated,
		Dropped:    turnDropped,
		TextCap:    turnTextCap,
		HasActive:  currentTurn != nil,
	}
}

// snapshotTurnsLocked 取「历史轮次 + 进行中轮次」的切片（需持有 streamMu）
func snapshotTurnsLocked() []*AgentTurn {
	out := make([]*AgentTurn, 0, len(turnHistory)+1)
	out = append(out, turnHistory...)
	if currentTurn != nil {
		out = append(out, currentTurn)
	}
	return out
}

// appendTurnText 追加轮次文本（超限保留尾部）
func appendTurnText(existing, addition string) string {
	merged := existing + addition
	if len(merged) > turnTextCap {
		merged = merged[len(merged)-turnTextCap:]
	}
	return merged
}

// emitStream 向 TUI 事件通道投递事件（通道未注册或已满时静默丢弃，轮次缓冲兜底）
func emitStream(evt StreamEvent) {
	streamChMu.RLock()
	ch := streamCh
	streamChMu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- evt:
	default:
	}
}

// SubscribeStream 注册 TUI 流式事件消费者（返回注销函数；TUI 退出时调用）
func SubscribeStream() (chan StreamEvent, func()) {
	streamChMu.Lock()
	defer streamChMu.Unlock()
	ch := make(chan StreamEvent, eventChanCap)
	streamCh = ch
	return ch, func() {
		streamChMu.Lock()
		defer streamChMu.Unlock()
		if streamCh == ch {
			streamCh = nil
		}
	}
}

// GetTurns 返回历史轮次 + 进行中轮次的渲染数据（旧 → 新）
func GetTurns() []*AgentTurn {
	streamMu.Lock()
	defer streamMu.Unlock()
	return snapshotTurnsLocked()
}

// GetTurnStoreInfo 返回轮次缓冲的容量与淘汰情况
func GetTurnStoreInfo() TurnStoreInfo {
	streamMu.Lock()
	defer streamMu.Unlock()
	return turnStoreInfoLocked()
}

// ==== 模型调用计量器（流式桥在每次调用完成时入账） ====

// RecordModelCall 入账一次模型调用：
// promptTokens/completionTokens 来自响应 usage（缺失时由调用方估算并置 estimated）。
func RecordModelCall(promptTokens, completionTokens int64, generation time.Duration, estimated bool) {
	meterMu.Lock()
	defer meterMu.Unlock()
	meterStats.Calls++
	meterStats.PromptTokens += promptTokens
	meterStats.CompletionTokens += completionTokens
	meterStats.GenerationNanos += int64(generation)
	if estimated {
		meterStats.EstimatedCalls++
	}
	meterStats.LastCallAt = time.Now()
	meterStats.LastDuration = generation
	meterStats.LastCompletion = completionTokens
}

// GetModelCallStats 返回计量数据副本
func GetModelCallStats() ModelCallStats {
	meterMu.Lock()
	defer meterMu.Unlock()
	return meterStats
}

// summarizeArgs 生成工具参数摘要（截断 + 去空白）
func summarizeArgs(args string) string {
	args = strings.TrimSpace(args)
	if len(args) > 120 {
		return args[:120] + "…"
	}
	return args
}
