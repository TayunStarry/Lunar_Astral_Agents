package napcat

// send_test.go 主动投递的传输层断言：请求体参数与上传回执超时语义。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// startSendFake 记录发送/上传请求体。
func startSendFake(t *testing.T, calls *[]map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	reply := func(w http.ResponseWriter, payload map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/send_group_msg", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		*calls = append(*calls, body)
		mu.Unlock()
		reply(w, map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{}})
	})
	mux.HandleFunc("/upload_group_file", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		*calls = append(*calls, body)
		mu.Unlock()
		reply(w, map[string]any{"status": "ok", "retcode": 0, "data": map[string]any{}})
	})
	return httptest.NewServer(mux)
}

// TestSendTextAndFileRequests 文本原样、文件走上传接口且带显示名与根目录。
func TestSendTextAndFileRequests(t *testing.T) {
	var calls []map[string]any
	server := startSendFake(t, &calls)
	defer server.Close()
	useTestClient(t, server.URL)

	if err := SendGroupPlainTextMessage(1001, "晚安，今天辛苦了"); err != nil {
		t.Fatalf("发送群文本失败: %v", err)
	}
	file := filepath.Join(t.TempDir(), "报告.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("写临时文件失败: %v", err)
	}
	if err := SendGroupFile(1001, file, "报告.txt"); err != nil {
		t.Fatalf("发送群文件失败: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("应调用发送接口 2 次，实际 %d", len(calls))
	}
	if calls[0]["group_id"] != float64(1001) {
		t.Fatalf("群号参数错误: %#v", calls[0])
	}
	segments := calls[0]["message"].([]any)
	text := segments[0].(map[string]any)["data"].(map[string]any)["text"]
	if text != "晚安，今天辛苦了" {
		t.Fatalf("文本应原样发送，实际 %#v", text)
	}
	if calls[1]["file"] != file || calls[1]["name"] != "报告.txt" {
		t.Fatalf("文件上传参数错误: %#v", calls[1])
	}
	if _, ok := calls[1]["folder"]; !ok {
		t.Fatalf("群文件应显式落在根目录: %#v", calls[1])
	}
}

// TestUnconfirmedUploadTreatedAsSuccess NapCat 已落盘但事件回执超时时按成功处理。
func TestUnconfirmedUploadTreatedAsSuccess(t *testing.T) {
	var calls []map[string]any
	server := startSendFake(t, &calls)
	defer server.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/upload_group_file", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "failed",
			"retcode": 100,
			"wording": "Timeout: NTEvent onMsgInfoListUpdate",
		})
	})
	timeoutServer := httptest.NewServer(mux)
	defer timeoutServer.Close()
	useTestClient(t, timeoutServer.URL)

	file := filepath.Join(t.TempDir(), "报告.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("写临时文件失败: %v", err)
	}
	if err := SendGroupFile(1001, file, "报告.txt"); err != nil {
		t.Fatalf("事件回执超时应按成功处理，实际失败: %v", err)
	}

	if !isUnconfirmedUpload(errors.New("Timeout: NTEvent onMsgInfoListUpdate")) {
		t.Fatal("应识别为已上传但未收到回执")
	}
	if isUnconfirmedUpload(errors.New("Timeout: NTEvent other")) {
		t.Fatal("非上传相关事件超时不应视为成功")
	}
}
