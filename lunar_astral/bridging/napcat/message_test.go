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

func TestParseMessageSegments_TextOnly(t *testing.T) {
	resetBridgeState()

	textData, _ := json.Marshal(TextData{Text: "你好世界"})
	segments := []MessageSegment{
		{Type: "text", Data: textData},
	}

	content, hasImages := parseMessageSegments(segments)

	if hasImages {
		t.Error("纯文本消息不应有图片")
	}
	if str, ok := content.(string); !ok || str != "你好世界" {
		t.Errorf("content = %v, 期望 '你好世界'", content)
	}
}

func TestParseMessageSegments_WithImage(t *testing.T) {
	resetBridgeState()

	textData, _ := json.Marshal(TextData{Text: "看这张图"})
	imageData, _ := json.Marshal(ImageData{URL: "https://example.com/img.jpg"})
	segments := []MessageSegment{
		{Type: "text", Data: textData},
		{Type: "image", Data: imageData},
	}

	content, hasImages := parseMessageSegments(segments)

	if !hasImages {
		t.Error("应检测到图片")
	}
	arr, ok := content.([]map[string]interface{})
	if !ok {
		t.Fatalf("content 应为 []map[string]interface{}, 实际为 %T", content)
	}
	if len(arr) != 2 {
		t.Fatalf("数组长度 = %d, 期望 2", len(arr))
	}
	// 第一个元素应为文本
	if arr[0]["type"] != "text" {
		t.Errorf("arr[0].type = %v, 期望 'text'", arr[0]["type"])
	}
	// 第二个元素应为 image_url
	if arr[1]["type"] != "image_url" {
		t.Errorf("arr[1].type = %v, 期望 'image_url'", arr[1]["type"])
	}
}

func TestExtractTextContent_String(t *testing.T) {
	result := extractTextContent("纯文本")
	if result != "纯文本" {
		t.Errorf("extractTextContent(string) = %q, 期望 '纯文本'", result)
	}
}

func TestExtractTextContent_Array(t *testing.T) {
	arr := []map[string]interface{}{
		{"type": "text", "text": "你好"},
		{"type": "image_url", "image_url": map[string]string{"url": "http://x"}},
		{"type": "text", "text": "世界"},
	}
	result := extractTextContent(arr)
	if result != "你好世界" {
		t.Errorf("extractTextContent(array) = %q, 期望 '你好世界'", result)
	}
}

func TestBuildOpenAIMessages_TextOnly(t *testing.T) {
	messages := []CachedMessage{
		{GroupID: 1, UserID: 100, Nickname: "Alice", Content: "你好", HasImages: false},
		{GroupID: 1, UserID: 200, Nickname: "Bob", Content: "月华在吗", HasImages: false},
	}

	result := buildOpenAIMessages(messages)
	if len(result) != 2 {
		t.Fatalf("消息数 = %d, 期望 2", len(result))
	}
	if result[0]["role"] != "user" {
		t.Error("role 应为 'user'")
	}
	if result[0]["content"] != "Alice : 你好" {
		t.Errorf("content = %v, 期望 'Alice : 你好'", result[0]["content"])
	}
}

func TestBuildOpenAIMessages_WithImages(t *testing.T) {
	contentArray := []map[string]interface{}{
		{"type": "text", "text": "看图"},
		{"type": "image_url", "image_url": map[string]string{"url": "http://x"}},
	}
	messages := []CachedMessage{
		{GroupID: 1, UserID: 100, Nickname: "Alice", Content: contentArray, HasImages: true},
	}

	result := buildOpenAIMessages(messages)
	if len(result) != 1 {
		t.Fatalf("消息数 = %d, 期望 1", len(result))
	}
	contentArr, ok := result[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content 应为 []map[string]interface{}, 实际为 %T", result[0]["content"])
	}
	// 第一项应为发送者标记
	if contentArr[0]["type"] != "text" || contentArr[0]["text"] != "Alice : " {
		t.Errorf("发送者标记不正确: %v", contentArr[0])
	}
	// 第二项应为原始文本
	if contentArr[1]["text"] != "看图" {
		t.Errorf("文本内容不正确: %v", contentArr[1])
	}
	// 第三项应为图片
	if contentArr[2]["type"] != "image_url" {
		t.Errorf("图片类型不正确: %v", contentArr[2])
	}
}

func TestHandleNapcatMessage_FilterSelfMessage(t *testing.T) {
	resetBridgeState()
	resetCache()

	msg := NapcatMessage{
		SelfID:      12345,
		UserID:      12345,
		GroupID:     262221051,
		MessageType: "group",
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)
	HandleNapcatMessage(raw)

	if GetCacheSize() != 0 {
		t.Error("自身消息应被过滤")
	}
}

func TestHandleNapcatMessage_FilterNonGroupMessage(t *testing.T) {
	resetBridgeState()
	resetCache()

	msg := NapcatMessage{
		SelfID:      12345,
		UserID:      67890,
		GroupID:     262221051,
		MessageType: "private",
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
		GroupID:     999999,
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

	SendMessageToAgent = nil
	HandleNapcatMessage(raw)

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

	callbackCalled := false
	var receivedMessages []map[string]interface{}
	SendMessageToAgent = func(messages []map[string]interface{}) {
		callbackCalled = true
		receivedMessages = messages
	}

	HandleNapcatMessage(raw)

	if !callbackCalled {
		t.Error("关键词触发后应调用SendMessageToAgent回调")
	}
	if len(receivedMessages) == 0 {
		t.Error("应收到消息")
	}
	if GetCacheSize() != 0 {
		t.Error("关键词触发推送后缓存应被清空")
	}
}

func TestHandleNapcatMessage_ImageMessage(t *testing.T) {
	resetBridgeState()
	resetCache()

	textData, _ := json.Marshal(TextData{Text: "月华看图"})
	imageData, _ := json.Marshal(ImageData{URL: "https://example.com/test.jpg"})
	msg := NapcatMessage{
		SelfID:  12345,
		UserID:  67890,
		GroupID: 262221051,
		Sender:  Sender{UserID: 67890, Nickname: "TestUser"},
		Message: []MessageSegment{
			{Type: "text", Data: textData},
			{Type: "image", Data: imageData},
		},
		MessageType: "group",
		PostType:    "message",
	}
	raw, _ := json.Marshal(msg)

	var receivedMessages []map[string]interface{}
	SendMessageToAgent = func(messages []map[string]interface{}) {
		receivedMessages = messages
	}

	HandleNapcatMessage(raw)

	if len(receivedMessages) == 0 {
		t.Fatal("应收到消息")
	}
	// 消息应为多模态格式
	contentArr, ok := receivedMessages[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("content 应为 []map[string]interface{}, 实际为 %T", receivedMessages[0]["content"])
	}
	// 应包含图片元素
	hasImage := false
	for _, item := range contentArr {
		if item["type"] == "image_url" {
			hasImage = true
			break
		}
	}
	if !hasImage {
		t.Error("多模态消息应包含 image_url 类型元素")
	}
}
