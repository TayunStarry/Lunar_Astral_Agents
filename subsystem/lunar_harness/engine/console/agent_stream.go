package console

// agent_stream.go 智能体流式事件与轮次缓冲的兼容入口。
// 状态与实现位于 logbuf 子包（debugsnap / server 直接依赖子包），
// 这里保留同名函数供 TUI 视图与既有调用方使用。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logbuf"
)

// StreamBegin 标记一次模型调用的开始（角色标签由请求头 X-Lunar-Role 提供）
func StreamBegin(role string) { logbuf.StreamBegin(role) }

// StreamDelta 追加一段流式增量（思考/输出）
func StreamDelta(role string, kind StreamKind, text string) { logbuf.StreamDelta(role, kind, text) }

// StreamTool 记录一次工具调用（聚合完成事件：名称 + 参数摘要）
func StreamTool(role, name, args string) { logbuf.StreamTool(role, name, args) }

// StreamToolResult 记录工具返回摘要
func StreamToolResult(role, summary string) { logbuf.StreamToolResult(role, summary) }

// StreamEnd 标记模型调用结束（落历史轮次）
func StreamEnd(role string) { logbuf.StreamEnd(role) }

// SubscribeStream 注册 TUI 流式事件消费者（返回注销函数；TUI 退出时调用）
func SubscribeStream() (chan StreamEvent, func()) { return logbuf.SubscribeStream() }

// GetTurns 返回历史轮次 + 进行中轮次的渲染数据（旧 → 新）
func GetTurns() []*AgentTurn { return logbuf.GetTurns() }

// GetTurnStoreInfo 返回轮次缓冲的容量与淘汰情况
func GetTurnStoreInfo() TurnStoreInfo { return logbuf.GetTurnStoreInfo() }

// RecordModelCall 入账一次模型调用
func RecordModelCall(promptTokens, completionTokens, cachedTokens int64, generation time.Duration, estimated bool) {
	logbuf.RecordModelCall(promptTokens, completionTokens, cachedTokens, generation, estimated)
}

// GetModelCallStats 返回计量数据副本
func GetModelCallStats() ModelCallStats { return logbuf.GetModelCallStats() }
