package napcat

// segments_test.go 出站消息段与资源字节获取的回归测试。

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// TestFetchFileBytesDataURI NapCat 可能直接把文件内容以 data URI 填在 url 字段
func TestFetchFileBytesDataURI(t *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G'}
	got, err := fetchFileBytes("data:image/png;base64," + base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("data URI 解码失败: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("data URI 解码结果不一致: %v", got)
	}
}

// TestBuildOutboundSegmentsQuotesFirst 引用段必须位于消息段首位（QQ 的硬性要求）
func TestBuildOutboundSegmentsQuotesFirst(t *testing.T) {
	segments := buildOutboundSegments(0, "是挺热的呀", 0, 9001)
	if len(segments) < 2 {
		t.Fatalf("消息段数量异常: %#v", segments)
	}
	if segments[0]["type"] != "reply" {
		t.Fatalf("引用段不在首位: %#v", segments)
	}
	data, _ := segments[0]["data"].(map[string]string)
	if data["id"] != "9001" {
		t.Fatalf("引用消息 ID 异常: %#v", data)
	}
}

// TestBuildOutboundSegmentsMentions 提及标记转真实 @ 段，回应 @ 与正文之间补空格。
func TestBuildOutboundSegmentsMentions(t *testing.T) {
	prev := Default()
	client := New(Config{})
	client.cacheMemberName(1001, 2001, "小明")
	SetDefault(client)
	t.Cleanup(func() { SetDefault(prev) })

	segments := buildOutboundSegments(1001, "[对 小明 说] 晚上好", 0, 0)
	if len(segments) != 2 {
		t.Fatalf("消息段数量异常: %#v", segments)
	}
	if segments[0]["type"] != "at" {
		t.Fatalf("首个消息段应为 @: %#v", segments[0])
	}
	if got := segments[0]["data"].(map[string]string)["qq"]; got != "2001" {
		t.Fatalf("@ 目标解析错误: %q", got)
	}
	if got := segments[1]["data"].(map[string]string)["text"]; got != " 晚上好" {
		t.Fatalf("正文应保留空格: %q", got)
	}
}
