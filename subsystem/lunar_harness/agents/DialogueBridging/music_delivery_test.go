package DialogueBridging

// music_delivery_test.go 演奏音频投递策略：语音条失败后必须发文字（含音频地址）并上传文件。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// startMusicFallbackFake 启动最小 NapCat 假服务：语音条返回失败，文本与文件接口成功。
func startMusicFallbackFake(t *testing.T, calls *[]map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	writeOK := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{}})
	}
	recordCall := func(action string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if action == "/send_group_msg" && isRecordMessage(body) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "failed", "retcode": 100, "wording": "mock record failure"})
				return
			}
			body["_action"] = action
			mu.Lock()
			*calls = append(*calls, body)
			mu.Unlock()
			writeOK(w)
		}
	}
	mux := http.NewServeMux()
	for _, action := range []string{"/send_group_msg", "/send_private_msg", "/upload_group_file", "/upload_private_file"} {
		mux.HandleFunc(action, recordCall(action))
	}
	return httptest.NewServer(mux)
}

// TestDeliverMusicFallsBackToTextAndFile 语音条失败后必须发文字（含音频地址）并上传文件。
func TestDeliverMusicFallsBackToTextAndFile(t *testing.T) {
	_ = setupMusicTest(t)
	var calls []map[string]any
	server := startMusicFallbackFake(t, &calls)
	defer server.Close()
	useNapcatTestClient(t, server.URL)

	err := deliverMusic(true, 1001, "/file/read/audios/test.wav", "测试小曲", "一段安静的测试旋律", 40.07)
	if err != nil {
		t.Fatalf("降级链路应成功，实际失败: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("应留下「文字 + 文件」2 次成功调用，实际 %d", len(calls))
	}
	if calls[0]["_action"] != "/send_group_msg" || isRecordMessage(calls[0]) {
		t.Fatalf("第一笔应为降级文字，实际 %#v", calls[0])
	}
	text := calls[0]["message"].([]any)[0].(map[string]any)["data"].(map[string]any)["text"].(string)
	if !strings.Contains(text, "改为发送音频文件") || !strings.Contains(text, "音频地址: http://127.0.0.1:18080/file/read/audios/test.wav") {
		t.Fatalf("降级文字缺少说明或音频地址: %q", text)
	}
	if calls[1]["_action"] != "/upload_group_file" {
		t.Fatalf("第二笔应为文件上传，实际 %#v", calls[1])
	}
	if calls[1]["file"] != filepath.Join(config.LocalDir, "audios", "test.wav") {
		t.Fatalf("文件上传路径错误: %#v", calls[1]["file"])
	}
}
