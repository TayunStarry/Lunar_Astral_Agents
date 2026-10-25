package napcat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// resetOutboundTest 备份/还原主动投递相关的全局状态
func resetOutboundTest(t *testing.T) {
	t.Helper()
	origConfig := bridgeConfig
	origState := GetBridgeState()
	origLoaded, origID, origNick := botInfoLoaded, botUserID, botNickname
	origAt, origGroups, origFriends, origErr := qqDirectoryAt, qqDirectoryGroups, qqDirectoryFriends, qqDirectoryErr
	t.Cleanup(func() {
		bridgeConfig = origConfig
		setBridgeState(origState)
		botInfoLoaded, botUserID, botNickname = origLoaded, origID, origNick
		qqDirectoryMu.Lock()
		qqDirectoryAt, qqDirectoryGroups, qqDirectoryFriends, qqDirectoryErr = origAt, origGroups, origFriends, origErr
		qqDirectoryMu.Unlock()
	})
}

// startOutboundFake 启动最小 NapCat 假服务：登录信息 / 群列表 / 好友列表 / 发送接口。
func startOutboundFake(t *testing.T, record *[]map[string]any) *httptest.Server {
	t.Helper()
	writeOK := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "retcode": 0, "data": data})
	}
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/get_login_info", func(w http.ResponseWriter, _ *http.Request) {
		writeOK(w, map[string]any{"user_id": 999, "nickname": "月华"})
	})
	mux.HandleFunc("/get_group_list", func(w http.ResponseWriter, _ *http.Request) {
		writeOK(w, []any{map[string]any{"group_id": 1001, "group_name": "测试群"}})
	})
	mux.HandleFunc("/get_friend_list", func(w http.ResponseWriter, _ *http.Request) {
		writeOK(w, []any{map[string]any{"user_id": 2001, "nickname": "小明", "remark": "同桌"}})
	})
	recordCall := func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		*record = append(*record, body)
		mu.Unlock()
		writeOK(w, map[string]any{})
	}
	mux.HandleFunc("/send_group_msg", recordCall)
	mux.HandleFunc("/upload_group_file", recordCall)
	return httptest.NewServer(mux)
}

// TestQQTargetsAndBridgeReady 钉住「连接成功且能解析群/好友信息」才允许主动投递。
func TestQQTargetsAndBridgeReady(t *testing.T) {
	resetOutboundTest(t)
	server := startOutboundFake(t, &[]map[string]any{})
	defer server.Close()
	bridgeConfig.BridgingPath = "ws://" + strings.TrimPrefix(server.URL, "http://")
	setBridgeState(BridgeConnected)
	botInfoLoaded, botUserID = false, 0
	qqDirectoryMu.Lock()
	qqDirectoryAt = time.Time{}
	qqDirectoryMu.Unlock()

	groups, friends, err := QQTargets()
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	if len(groups) != 1 || groups[0].GroupID != 1001 || groups[0].GroupName != "测试群" {
		t.Fatalf("群信息解析错误: %#v", groups)
	}
	if len(friends) != 1 || friends[0].UserID != 2001 || friends[0].Remark != "同桌" {
		t.Fatalf("好友信息解析错误: %#v", friends)
	}
	if !BridgeReady() {
		t.Fatal("连接成功且目录可用时应判定为就绪")
	}
}

// TestOutboundSendTextAndFile 钉住主动投递：文本原样、文件走上传接口且带显示名。
func TestOutboundSendTextAndFile(t *testing.T) {
	resetOutboundTest(t)
	var calls []map[string]any
	server := startOutboundFake(t, &calls)
	defer server.Close()
	bridgeConfig.BridgingPath = "ws://" + strings.TrimPrefix(server.URL, "http://")

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
	if calls[1]["file"] != file || calls[1]["name"] != "报告.txt" {
		t.Fatalf("文件上传参数错误: %#v", calls[1])
	}
}
