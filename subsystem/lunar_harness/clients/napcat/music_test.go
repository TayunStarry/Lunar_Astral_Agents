package napcat

// music_test.go QQ 语音条出站的干跑验证：
// 用假 NapCat HTTP 服务捕获消息段，确认 record 段结构与音频路径归一。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// setupMusicTest 准备临时内容仓库并注入指向假服务的客户端。
func setupMusicTest(t *testing.T) string {
	t.Helper()
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
	for _, action := range []string{"/send_group_msg", "/send_private_msg"} {
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

// useTestClient 注入指向假服务的默认客户端，测试结束还原。
func useTestClient(t *testing.T, serverURL string) {
	t.Helper()
	prev := Default()
	SetDefault(New(Config{Endpoint: "ws://" + strings.TrimPrefix(serverURL, "http://")}))
	t.Cleanup(func() { SetDefault(prev) })
}

// TestSendGroupRecordMessageUsesLocalAudio 语音条引用本机内容仓库绝对路径。
func TestSendGroupRecordMessageUsesLocalAudio(t *testing.T) {
	setupMusicTest(t)
	var calls []map[string]any
	server := startMusicFake(t, &calls)
	defer server.Close()
	useTestClient(t, server.URL)

	if err := SendGroupRecordMessage(1001, "/file/read/audios/test.wav"); err != nil {
		t.Fatalf("发送群语音条失败: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("应调用 1 次发送接口，实际 %d", len(calls))
	}
	segments := calls[0]["message"].([]any)
	segment := segments[0].(map[string]any)
	if segment["type"] != "record" {
		t.Fatalf("消息段类型应为 record，实际 %#v", segment["type"])
	}
	file := segment["data"].(map[string]any)["file"].(string)
	if !filepath.IsAbs(file) {
		t.Fatalf("语音条应使用本机绝对路径，实际 %q", file)
	}
	if info, err := os.Stat(file); err != nil || info.Size() == 0 {
		t.Fatalf("语音条引用的音频不可读: %v", err)
	}
}
