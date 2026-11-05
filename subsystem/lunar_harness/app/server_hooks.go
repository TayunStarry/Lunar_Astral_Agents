package app

// server_hooks.go 装配层向 HTTP 层注入跨层能力。
//
// server 只声明函数变量（见 server/variable.go），由这里把编排层与智能体实现接上，
// 保证 HTTP 层不 import chain/* 与 agents/Responder。

import (
	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/chain/thinking"
	"LunarSubsystem/LunarHarness/server"
)

// wireServerHooks 在 server.Start 之前完成全部注入。
func wireServerHooks() {
	server.MessageBatchHandlerFunc = thinking.MessageBatchHandler
	server.ProxyHandlerFunc = thinking.ProxyHandler
	server.VisionReadFunc = thinking.VisionReadBlob
	server.AgentPendingFunc = Responder.Pending
	server.AgentEnqueueFunc = enqueueExternalAgent
}

// enqueueExternalAgent 把 server 的排队请求适配为响应者智能体的入队请求，
// 并把异步回执转回 HTTP 层使用的结构（两者字段一致，仅类型不同）。
func enqueueExternalAgent(req *server.AgentRequest) error {
	inner := &Responder.Request{
		Messages: req.Messages,
		Params:   req.Params,
		Reply:    make(chan Responder.Result, 1),
	}
	if err := Responder.Enqueue(inner); err != nil {
		return err
	}
	go func() {
		result := <-inner.Reply
		converted := server.AgentResult{
			Content:      result.Content,
			Reasoning:    result.Reasoning,
			ToolCalls:    result.ToolCalls,
			FinishReason: result.FinishReason,
			Err:          result.Err,
		}
		// 请求超时后 handler 已返回，此时丢弃结果即可（缓冲为 1，正常路径不会阻塞）
		select {
		case req.Reply <- converted:
		default:
		}
	}()
	return nil
}
