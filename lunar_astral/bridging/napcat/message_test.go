package napcat

import (
	"encoding/json"
	"testing"
)

// resetBridgeState 重置桥接器状态
func resetBridgeState() {
	bridgeConfig = BridgingConfig{
		BridgingType:     "napcat",
		BridgingPath:     "ws://localhost:4567",
		BridgingToken:    "test-token",
		BridgingTarget:   262221051,
		BridgingKeywords: []string{"月华", "3826713076"},
	}
	setBridgeState(BridgeDisconnected)
	scanMutex.Lock()
	scanRetryCount = 0
	if scanTimer != nil {
		scanTimer.Stop()
		scanTimer = nil
	}
	scanMutex.Unlock()
}

func TestContainsKeyword_Match(t *testing.T) {
	resetBridgeState()
	resetCache()

	if !containsKeyword("你好月华") {
		t.Error("包含关键词'月华'应返回true")
	}
	if !containsKeyword("3826713076在线吗") {
		t.Error("包含关键词'3826713076'应返回true")
	}
}

func TestContainsKeyword_NoMatch(t *testing.T) {
	resetBridgeState()
	resetCache()

	if containsKeyword("今天天气真好") {
		t.Error("不包含任何关键词应返回false")
	}
}

func TestContainsKeyword_EmptyKeywords(t *testing.T) {
	resetBridgeState()
	resetCache()

	bridgeConfig.BridgingKeywords = nil
	if !containsKeyword("任意内容") {
		t.Error("关键词为空时应始终返回true")
	}
}

func TestBuildPushContent(t *testing.T) {
	messages := []CachedMessage{
		{GroupID: 1, UserID: 100, Nickname: "Alice", Content: "你好"},
		{GroupID: 1, UserID: 200, Nickname: "Bob", Content: "月华在吗"},
	}

	result := buildPushContent(messages)
	expected := "Alice: 你好\nBob: 月华在吗"
	if result != expected {
		t.Errorf("buildPushContent = %q, 期望 %q", result, expected)
	}
}

func TestHandleNapcatMessage_FilterSelfMessage(t *testing.T) {
	resetBridgeState()
	resetCache()

	// 自己发送的消息应被过滤
	msg := NapcatMessage{
		SelfID:      12345,
		UserID:      12345, // 与SelfID相同
		GroupID:     262221051,
		MessageType: "group",
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)

	HandleNapcatMessage(raw)

	// 缓存应为空
	if GetCacheSize() != 0 {
		t.Error("自身消息应被过滤，不应缓存")
	}
}

func TestHandleNapcatMessage_FilterNonGroupMessage(t *testing.T) {
	resetBridgeState()
	resetCache()

	msg := NapcatMessage{
		SelfID:      12345,
		UserID:      67890,
		GroupID:     262221051,
		MessageType: "private", // 非群消息
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)

	HandleNapcatMessage(raw)

	if GetCacheSize() != 0 {
		t.Error("非群消息应被过滤")
	}
}

func TestHandleNapcatMessage_FilterWrongGroup(t *testing.T) {
	resetBridgeState()
	resetCache()

	msg := NapcatMessage{
		SelfID:      12345,
		UserID:      67890,
		GroupID:     999999, // 非目标群
		MessageType: "group",
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)

	HandleNapcatMessage(raw)

	if GetCacheSize() != 0 {
		t.Error("非目标群消息应被过滤")
	}
}

func TestHandleNapcatMessage_ValidMessageNoKeyword(t *testing.T) {
	resetBridgeState()
	resetCache()

	textData, _ := json.Marshal(TextData{Text: "普通聊天内容"})
	msg := NapcatMessage{
		SelfID:  12345,
		UserID:  67890,
		GroupID: 262221051,
		Sender:  Sender{UserID: 67890, Nickname: "TestUser"},
		Message: []MessageSegment{
			{Type: "text", Data: textData},
		},
		MessageType: "group",
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)

	// 未注册回调时不应panic
	SendMessageToAgent = nil
	HandleNapcatMessage(raw)

	// 消息应被缓存
	if GetCacheSize() != 1 {
		t.Errorf("普通消息应被缓存, 缓存大小 = %d, 期望 1", GetCacheSize())
	}
}

func TestHandleNapcatMessage_KeywordTrigger(t *testing.T) {
	resetBridgeState()
	resetCache()

	textData, _ := json.Marshal(TextData{Text: "月华在吗？"})
	msg := NapcatMessage{
		SelfID:  12345,
		UserID:  67890,
		GroupID: 262221051,
		Sender:  Sender{UserID: 67890, Nickname: "TestUser"},
		Message: []MessageSegment{
			{Type: "text", Data: textData},
		},
		MessageType: "group",
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)

	// 注册回调，验证是否被调用
	callbackCalled := false
	var receivedContent string
	SendMessageToAgent = func(content string, senderName string) {
		callbackCalled = true
		receivedContent = content
	}

	HandleNapcatMessage(raw)

	if !callbackCalled {
		t.Error("关键词触发后应调用SendMessageToAgent回调")
	}
	if receivedContent == "" {
		t.Error("推送内容不应为空")
	}
	// 推送后缓存应被清空
	if GetCacheSize() != 0 {
		t.Error("关键词触发推送后缓存应被清空")
	}
}
