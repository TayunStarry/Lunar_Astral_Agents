package Screenshoter

// type.go 截图者的类型定义。

import (
	"context"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Env 宿主能力注入。
//
// 截图者只负责「截什么、怎么截、截到了什么」，展示与落盘属于对话者所在的宿主层。
type Env interface {
	// PushImages 把截图推送给两个前端，返回规范化后的可查看地址
	PushImages(images []string) []string
	// SaveImage 落盘一帧截图并返回可查看地址
	SaveImage(name string, data []byte) (string, error)
}

// Result 一次截图任务的结果。
type Result struct {
	// Summary 回传给对话者的自然语言汇报
	Summary string
	// Image 主帧的 base64 数据（供对话者做视觉副本）
	Image string
}

// shot 一张已完成的截图（跨轮次汇总用）。
type shot struct {
	Note        string
	Mode        string
	WindowTitle string
	Width       int
	Height      int
	Base64      string
	URL         string
}

// toolCall 一次工具调用的函数名与原始参数。
type toolCall struct {
	Name      string
	Arguments string
}

// baseMessage 截图者请求中的消息项（统一复用基座类型）。
type baseMessage = agentbase.Message

// runtimeState 跨任务保留的截图会话上下文。
type runtimeState struct {
	history []baseMessage
}

// session 单次截图任务的编排状态。
type session struct {
	ctx     context.Context
	env     Env
	system  string
	history []baseMessage
	pending *shot
	shots   []shot
	primary string
}

// trimHistory 保留系统消息与最近 limit 条历史，避免上下文无限增长。
func trimHistory(messages []baseMessage, limit int) []baseMessage {
	if len(messages) <= limit+1 {
		return messages
	}
	out := make([]baseMessage, 0, limit+1)
	out = append(out, messages[0])
	out = append(out, messages[len(messages)-limit:]...)
	return out
}
