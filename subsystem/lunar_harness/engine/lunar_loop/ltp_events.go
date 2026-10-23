package lunarloop

// ==== LTP9 事件触发点（Go 侧） ====
// 事件统一在 Go 边界发起，goja 对话链不再发送或处理事件：
//   - 读取消息前：pullContext 把即将交付的消息交给插件改写（清空/追加/移除/重排）；
//   - 消息发送前：pushContext 把即将发出的消息交给插件改写、追加或取消；
//   - 查询前后：宿主 web_search 分支触发（见 crystal/ltp9_events.go）。
// 无插件订阅、未回传业务结果或回传结构非法时，一律沿用原始数据。

import (
	"encoding/json"

	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

// dispatchBeforeReadingMessage 读取消息前事件（before_reading_message）：
// 插件可返回 { messages } 替换本轮消息；返回空数组即清空本轮。
func dispatchBeforeReadingMessage(messages []PostMessage) []PostMessage {
	if len(messages) == 0 {
		return messages
	}
	res := StarLTP.Emit(StarLTP.EventBeforeReadingMessage, map[string]any{"messages": messages}, "")
	feedback, ok := res.Return.(map[string]any)
	if !res.Returned || !ok {
		return messages
	}
	raw, has := feedback["messages"]
	if !has {
		return messages
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return messages
	}
	var out []PostMessage
	if err := json.Unmarshal(data, &out); err != nil {
		return messages
	}
	return out
}

// dispatchBeforeSendingMessage 消息发送前事件（before_sending_amessage）：
// 插件可返回 { messages } 改写/追加待发消息，或 { cancel: true } 取消本次发送。
// 返回值 ok=false 表示取消发送；改写为空列表同样视为取消。
func dispatchBeforeSendingMessage(items []PushContextData) ([]PushContextData, bool) {
	if len(items) == 0 {
		return items, true
	}
	res := StarLTP.Emit(StarLTP.EventBeforeSendingMessage, map[string]any{"messages": items}, "")
	feedback, ok := res.Return.(map[string]any)
	if !res.Returned || !ok {
		return items, true
	}
	if cancel, _ := feedback["cancel"].(bool); cancel {
		return nil, false
	}
	raw, has := feedback["messages"]
	if !has {
		return items, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return items, true
	}
	var out []PushContextData
	if err := json.Unmarshal(data, &out); err != nil {
		return items, true
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
