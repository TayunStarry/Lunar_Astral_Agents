package napcat

// music_test.go QQ 音乐出站的干跑验证：
// 用假 NapCat HTTP 服务捕获消息段，确认语音条结构、失败降级与文件上传参数。

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

// startMusicFake 启动最小 NapCat 假服务；failRecord=true 时语音条返回失败，其余接口成功。
func startMusicFake(t *testing.T, failRecord bool, calls *[]map[string]any) *httptest.Server {
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
			if failRecord && action == "/send_group_msg" && isRecordMessage(body) {
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
	server := startMusicFake(t, false, &calls)
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

// TestSendMusicFallsBackToTextAndFile 语音条失败后必须发文字（含音频地址）并上传文件。
func TestSendMusicFallsBackToTextAndFile(t *testing.T) {
	setupMusicTest(t)
	var calls []map[string]any
	server := startMusicFake(t, true, &calls)
	defer server.Close()
	useTestClient(t, server.URL)

	err := SendMusic(true, 1001, "/file/read/audios/test.wav", "测试小曲", "一段安静的测试旋律", 40.07)
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
