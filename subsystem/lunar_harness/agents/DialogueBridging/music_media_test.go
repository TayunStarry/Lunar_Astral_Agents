package DialogueBridging

// music_media_test.go 音乐出站的会话门控：无 QQ 请求时不得向 QQ 推送。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func setupMusicTest(t *testing.T) string {
	t.Helper()
	resetOutboundTest(t)
	tmp := t.TempDir()
	origLocal, origPort := config.LocalDir, config.BasicPort
	config.LocalDir = tmp
	config.BasicPort = 18080
	t.Cleanup(func() {
		config.LocalDir, config.BasicPort = origLocal, origPort
	})
	if err := os.MkdirAll(filepath.Join(tmp, "audios"), 0o755); err != nil {
		t.Fatalf("准备音频目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "audios", "test.wav"), []byte("RIFF-test-audio"), 0o644); err != nil {
		t.Fatalf("写入测试音频失败: %v", err)
	}
	return tmp
}

// startMusicFake 启动最小 NapCat 假服务并记录发送调用。
func startMusicFake(t *testing.T, calls *[]map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	mux := http.NewServeMux()
	for _, action := range []string{"/send_group_msg", "/send_private_msg", "/upload_group_file", "/upload_private_file"} {
		action := action
		mux.HandleFunc(action, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["_action"] = action
			mu.Lock()
			*calls = append(*calls, body)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{}})
		})
	}
	return httptest.NewServer(mux)
}

func isRecordMessage(body map[string]any) bool {
	segments, ok := body["message"].([]any)
	if !ok || len(segments) == 0 {
		return false
	}
	first, _ := segments[0].(map[string]any)
	return first["type"] == "record"
}

// TestHandleAgentMusicResponseOnlyForQQ 无 QQ 请求时不得向 QQ 推送；有请求时才发送。
func TestHandleAgentMusicResponseOnlyForQQ(t *testing.T) {
	_ = setupMusicTest(t)
	var calls []map[string]any
	server := startMusicFake(t, &calls)
	defer server.Close()
	useNapcatTestClient(t, server.URL)
	setBridgeState(BridgeConnected)
	t.Cleanup(func() { setBridgeState(BridgeDisconnected) })

	flowMutex.Lock()
	origAwaiting, origStarted, origTarget := awaitingResponse, responseStarted, currentTarget
	awaitingResponse = false
	responseStarted = false
	flowMutex.Unlock()
	t.Cleanup(func() {
		flowMutex.Lock()
		awaitingResponse, responseStarted, currentTarget = origAwaiting, origStarted, origTarget
		flowMutex.Unlock()
	})

	HandleAgentMusicResponse("/file/read/audios/test.wav", "测试小曲", "描述", 40.07)
	if len(calls) != 0 {
		t.Fatalf("Web 会话（无 QQ 请求）不应发送，实际调用 %d 次", len(calls))
	}

	flowMutex.Lock()
	awaitingResponse = true
	responseStarted = false
	currentTarget = BridgeTarget{ID: 2001, IsGroup: false}
	flowMutex.Unlock()

	HandleAgentMusicResponse("/file/read/audios/test.wav", "测试小曲", "描述", 40.07)
	if len(calls) != 1 {
		t.Fatalf("QQ 私聊请求应发送 1 次，实际 %d", len(calls))
	}
	if calls[0]["_action"] != "/send_private_msg" || !isRecordMessage(calls[0]) {
		t.Fatalf("QQ 私聊应走语音条，实际 %#v", calls[0])
	}
	if calls[0]["user_id"] != float64(2001) {
		t.Fatalf("私聊目标错误: %#v", calls[0]["user_id"])
	}
	flowMutex.Lock()
	gotStarted := responseStarted
	flowMutex.Unlock()
	if gotStarted {
		t.Fatal("音乐在工具轮次中途产出，不应置位 responseStarted（终稿文本才收尾）")
	}
}
