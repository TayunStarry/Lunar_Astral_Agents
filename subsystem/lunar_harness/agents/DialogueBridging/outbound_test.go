package DialogueBridging

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// resetOutboundTest 备份/还原主动投递相关的业务全局状态。
func resetOutboundTest(t *testing.T) {
	t.Helper()
	origConfig := bridgeConfig
	origState := GetBridgeState()
	resetWhitelistPending()
	t.Cleanup(func() {
		resetWhitelistPending()
		bridgeConfig = origConfig
		setBridgeState(origState)
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
	useNapcatTestClient(t, server.URL)
	setBridgeState(BridgeConnected)

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

// TestQQTargetsRequiresConnection 桥接器未连接时不得读取目录。
func TestQQTargetsRequiresConnection(t *testing.T) {
	resetOutboundTest(t)
	setBridgeState(BridgeDisconnected)
	if _, _, err := QQTargets(); err == nil {
		t.Fatal("未连接时应返回错误")
	}
}
