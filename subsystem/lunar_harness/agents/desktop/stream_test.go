//go:build windows

package desktop

// 桌面操纵的模型调用：实况推送、工具调用分片聚合与思考监管。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// 覆盖：SSE 分片被正确聚合为正文 / 工具调用 / 用量，并同步到实况。
func TestReadAutoStreamAggregatesChunks(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	guard := loopguard.New(cancel, loopguard.DefaultOptions())

	var think, content strings.Builder
	SetModelObserver(
		func(string) {},
		func(_, kind, text string) {
			if kind == streamKindThink {
				think.WriteString(text)
			} else {
				content.WriteString(text)
			}
		},
		func(string) {},
		nil,
	)
	defer SetModelObserver(nil, nil, nil, nil)

	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"先想想"}}]}`,
		`data: {"choices":[{"delta":{"content":"你好"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"click","arguments":"{\"x\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"10,\"y\":20}"}}]}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":34}}`,
		`data: [DONE]`,
		"",
	}, "\n")

	msg, usage, err := readAutoStream(strings.NewReader(sse), guard, "测试")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if messageText(msg.Content) != "你好" {
		t.Fatalf("正文聚合错误: %q", messageText(msg.Content))
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("工具调用应聚合为 1 个，实得 %d", len(msg.ToolCalls))
	}
	call := msg.ToolCalls[0]
	if call.Function.Name != "click" || call.Function.Arguments != `{"x":10,"y":20}` {
		t.Fatalf("工具调用聚合错误: %+v", call)
	}
	if usage == nil || usage.Prompt != 12 || usage.Completion != 34 {
		t.Fatalf("用量解析错误: %+v", usage)
	}
	if think.String() != "先想想" || content.String() != "你好" {
		t.Fatalf("实况增量错误: think=%q content=%q", think.String(), content.String())
	}
}

// 覆盖：模型重复输出时由思考监管中断并重试，超过上限后如实失败。
func TestAutoChatRetriesLoopThenFails(t *testing.T) {
	requests := 0
	repeated := strings.Repeat("重复内容", 300) // 1200 字，跨过一个 1024 判定节点
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", repeated)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	config.AgentMultimodalURL = srv.URL
	config.AgentMultimodalModel = "test-loop"

	_, err := autoChat(context.Background(), autoRoleEditor, []chatMessage{{Role: "user", Content: "hi"}}, nil)
	if !errors.Is(err, loopguard.ErrLoop) {
		t.Fatalf("应返回 loopguard.ErrLoop，实得 %v", err)
	}
	want := autoLoopMaxRetries + 1
	if requests != want {
		t.Fatalf("应请求 %d 次（含重试），实得 %d", want, requests)
	}
}
