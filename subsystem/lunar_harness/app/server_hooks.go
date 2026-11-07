package app

// server_hooks.go 装配层向 HTTP 层注入跨层能力。
//
// server 只声明函数变量（见 server/variable.go），由这里把编排层与智能体实现接上，
// 保证 HTTP 层不 import chain/* 与 agents/Responder。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/chain/thinking"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/server"
)

// wireServerHooks 在 server.Start 之前完成全部注入。
func wireServerHooks() {
	server.MessageBatchHandlerFunc = thinking.MessageBatchHandler
	server.ProxyHandlerFunc = thinking.ProxyHandler
	server.VisionReadFunc = thinking.VisionReadBlob
	server.AgentPendingFunc = Responder.Pending
	server.AgentEnqueueFunc = enqueueExternalAgent
	server.AgentPrepareMessagesFunc = prepareAgentMessages
}

// agentEnvelopeTypes 需要经过信封接收链路的内容类型（六类新信封 + 四类已废止旧信封）。
// 命旧类型时同样进入链路：由接收侧判定无效并记录丢弃原因。
var agentEnvelopeTypes = map[string]bool{
	mediaobject.EnvelopeImageStatic:   true,
	mediaobject.EnvelopeImageAnimated: true,
	mediaobject.EnvelopeVideo:         true,
	mediaobject.EnvelopeAudio:         true,
	mediaobject.EnvelopeFile:          true,
	mediaobject.EnvelopeWebpage:       true,
	mediaobject.EnvelopeImageURL:      true,
	mediaobject.EnvelopeVideoURL:      true,
	mediaobject.EnvelopeAudioURL:      true,
	mediaobject.EnvelopeInputAudio:    true,
}

// prepareAgentMessages 把外部推理请求里的多模态内容送进统一信封链路。
//
// 纯文本/工具消息原样返回，避免无谓的 JSON 重编码丢失调用方字段；
// 只有出现媒体内容项时才解析为 PostMessage 并做「校验 → 落盘 → 清洗」。
// 结构非法时直接报错：无法判断是否含媒体就放行，等于让请求绕过信封协议。
func prepareAgentMessages(raw json.RawMessage) (json.RawMessage, error) {
	hasMedia, err := hasAgentEnvelopeContent(raw)
	if err != nil {
		return nil, err
	}
	if !hasMedia {
		return raw, nil
	}
	var messages []thinking.PostMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, fmt.Errorf("messages 解析失败: %v", err)
	}
	converted, err := json.Marshal(thinking.ResourcesConversionHandle(messages))
	if err != nil {
		return nil, fmt.Errorf("messages 清洗失败: %v", err)
	}
	return converted, nil
}

// hasAgentEnvelopeContent 判断消息数组里是否含媒体内容项。
//
// content 的合法形态只有三种：缺省、null、字符串（纯文本），或内容对象数组。
// 其余形态（数字/布尔/裸对象、数组里出现非对象元素）无法判定是否含媒体，
// 一律返回错误，交给上层回 400，不再静默透传。
func hasAgentEnvelopeContent(raw json.RawMessage) (bool, error) {
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		return false, fmt.Errorf("messages 结构无效（应为消息对象数组）: %v", err)
	}
	if len(messages) == 0 {
		return false, errors.New("messages 不能为空")
	}
	for index, message := range messages {
		hasMedia, err := messageHasEnvelope(message.Content)
		if err != nil {
			return false, fmt.Errorf("第 %d 条消息的 content 非法: %v", index+1, err)
		}
		if hasMedia {
			return true, nil
		}
	}
	return false, nil
}

// messageHasEnvelope 检查单条消息的 content 是否含媒体内容项。
func messageHasEnvelope(content json.RawMessage) (bool, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false, nil
	}
	if trimmed[0] == '"' {
		return false, nil
	}
	if trimmed[0] != '[' {
		return false, errors.New("只允许字符串、null 或内容对象数组")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return false, fmt.Errorf("不是合法数组: %v", err)
	}
	for index, item := range items {
		entry := bytes.TrimSpace(item)
		if len(entry) == 0 || entry[0] != '{' {
			return false, fmt.Errorf("第 %d 项不是内容对象", index+1)
		}
		var item struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(entry, &item); err != nil {
			return false, fmt.Errorf("第 %d 项解析失败: %v", index+1, err)
		}
		if agentEnvelopeTypes[item.Type] {
			return true, nil
		}
	}
	return false, nil
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
