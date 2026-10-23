package napcat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"LunarSubsystem/GeneralConfig"
	file "LunarSubsystem/LunarHarness/engine/persistence"
)

// TestDataURIExt 合并转发内的图片常常只有 file 引用，get_image 下载后是 data URI。
// 归档层按扩展名推断类型，data URI 必须补上正确扩展名，否则会被当成文档。
func TestDataURIExt(t *testing.T) {
	cases := map[string]string{
		"data:image/png;base64,AAAA":  ".png",
		"data:image/jpeg;base64,AAAA": ".jpg",
		"data:image/jpg;base64,AAAA":  ".jpg",
		"data:image/gif;base64,AAAA":  ".gif",
		"data:image/webp;base64,AAA":  ".webp",
		"data:image/bmp;base64,AAAA":  ".bmp",
		"data:image/avif;base64,AAA":  ".png", // 未知格式回退 png
		"data:,":                      ".png",
	}
	for uri, want := range cases {
		if got := dataURIExt(uri); got != want {
			t.Fatalf("dataURIExt(%q) = %q, want %q", uri, got, want)
		}
	}
}

// TestForwardImageName QQ CDN 直链常常没有扩展名；没有图片扩展名时归档层会按文档处理。
func TestForwardImageName(t *testing.T) {
	cases := map[string]string{
		"https://multimedia.nt.qq.com.cn/download?fileid=abc": "forward.jpg",
		"https://example.com/a.png":                           "forward.png",
		"https://example.com/a.JPEG?x=1":                      "forward.jpeg",
		"data:image/gif;base64,AAA":                           "forward.gif",
	}
	for source, want := range cases {
		if got := forwardImageName(source); got != want {
			t.Fatalf("forwardImageName(%q) = %q, want %q", source, got, want)
		}
	}
}

// TestAdmitForwardImageDataURI 只有 file 引用、下载后为 data URI 的转发图片
// 必须能收进资源归档并返回句柄，而不是退化成 [图片]。
func TestAdmitForwardImageDataURI(t *testing.T) {
	origLocal := *GeneralConfig.LocalDir
	origArchive := *GeneralConfig.ArchiveDBPath
	tmp := t.TempDir()
	*GeneralConfig.LocalDir = tmp
	*GeneralConfig.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		_ = file.CloseArchiveDB()
		*GeneralConfig.LocalDir = origLocal
		*GeneralConfig.ArchiveDBPath = origArchive
	})

	const onePixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	handle, ok := admitForwardImage(ImageData{URL: onePixelPNG})
	if !ok || handle == "" {
		t.Fatal("data URI 转发图片入库失败，会退化成 [图片]")
	}
	if handle[0] != '@' || handle[1] != 'I' {
		t.Fatalf("转发图片应返回图片句柄，实得 %q", handle)
	}

	if _, ok := admitForwardImage(ImageData{}); ok {
		t.Fatal("空图片数据不应返回句柄")
	}
}

// ==== A@B 私聊过滤与引用月华触发 ====

const testSelfID = int64(999)

// fakeNapcat 最小 NapCat HTTP 假服务：应答登录/群信息/成员列表/消息详情。
type fakeNapcat struct {
	server       *httptest.Server
	quotedSender int64 // get_msg 返回的被引用消息发送者
}

func startFakeNapcat(t *testing.T, selfID int64) *fakeNapcat {
	fake := &fakeNapcat{}
	writeOK := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "retcode": 0, "data": data})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/get_login_info", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]any{"user_id": selfID, "nickname": "月华"})
	})
	mux.HandleFunc("/get_group_info", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]any{"group_name": "测试群"})
	})
	mux.HandleFunc("/get_group_member_list", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, []any{})
	})
	mux.HandleFunc("/get_msg", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]any{"sender": map[string]any{"user_id": fake.quotedSender}})
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// startBridgeTest 替换桥接配置、消息回调与全局缓存（测试结束后还原），
// 并把桥接地址指向假 NapCat 服务。返回假服务与送达计数指针。
func startBridgeTest(t *testing.T, groupID int64) (*fakeNapcat, *int) {
	fake := startFakeNapcat(t, testSelfID)
	zero := 0.0
	origConfig := bridgeConfig
	origSender := SendMessageToAgent
	origVideo := SendVideoToAgent
	origAudio := SendAudioToAgent
	origPools := groupPools
	origWarmed := memberListWarmed
	origNames := groupNameCache
	origMemberNames := memberNameCache
	origBotLoaded := botInfoLoaded
	origAwaiting := awaitingResponse
	origStarted := responseStarted
	origQueue := requestQueue
	t.Cleanup(func() {
		bridgeConfig = origConfig
		SendMessageToAgent = origSender
		SendVideoToAgent = origVideo
		SendAudioToAgent = origAudio
		groupPools = origPools
		memberListWarmed = origWarmed
		groupNameCache = origNames
		memberNameCache = origMemberNames
		botInfoLoaded = origBotLoaded
		flowMutex.Lock()
		awaitingResponse = origAwaiting
		responseStarted = origStarted
		requestQueue = origQueue
		flowMutex.Unlock()
	})
	bridgeConfig = BridgingConfig{
		BridgingType:                    "napcat",
		BridgingPath:                    "ws://" + fake.server.Listener.Addr().String(),
		BridgingUsers:                   []int64{groupID},
		BridgingGroupTriggerProbability: &zero,
	}
	delivered := 0
	SendMessageToAgent = func(messages []map[string]any) { delivered++ }
	SendVideoToAgent = func(urls []string) {}
	SendAudioToAgent = func(urls []string) {}
	// 重置登录信息缓存：让 resolveAtDisplay 走假服务拿到测试用 selfID
	botInfoLoaded = false
	return fake, &delivered
}

// groupMsg 构造一条群聊消息（segments 为消息段）
func groupMsg(groupID, userID int64, segments ...MessageSegment) NapcatMessage {
	return NapcatMessage{
		SelfID:      testSelfID,
		UserID:      userID,
		GroupID:     groupID,
		PostType:    "message",
		MessageType: "group",
		Message:     segments,
	}
}

func atSeg(qq string) MessageSegment {
	return MessageSegment{Type: "at", Data: json.RawMessage(`{"qq":"` + qq + `"}`)}
}

func textSeg(text string) MessageSegment {
	raw, _ := json.Marshal(map[string]string{"text": text})
	return MessageSegment{Type: "text", Data: raw}
}

func replySeg(id string) MessageSegment {
	return MessageSegment{Type: "reply", Data: json.RawMessage(`{"id":"` + id + `"}`)}
}

// deliverAndSettle 投递一条群消息并模拟月华完成应答。
// pumpNext 推送时持有 flowMutex 且会置位等待状态，只能在消息之间
// 走真实的应答收尾链路（HandleAgentResponse + NotifyAgentIdle）推进流程。
func deliverAndSettle(msg NapcatMessage) {
	handleGroupMessage(msg)
	HandleAgentResponse("text", "好哒", false)
	NotifyAgentIdle()
}

// TestHandleGroupMessagePrivateAtDrop 钉住 A@B 私聊过滤语义：
// 直接 @ 其他成员（不含月华）的消息与月华无关，不入池也不触发，
// 关键词与随机概率都不得把月华拉进成员间的私聊。
func TestHandleGroupMessagePrivateAtDrop(t *testing.T) {
	const group = int64(7700001)
	_, delivered := startBridgeTest(t, group)

	// 纯 A@B：A(111) @ B(222)
	deliverAndSettle(groupMsg(group, 111, atSeg("222"), textSeg("今晚一起吃饭吗")))
	if *delivered != 0 {
		t.Fatalf("A@B 私聊消息不应送达月华")
	}
	if got := len(snapshotGroupPool(group)); got != 0 {
		t.Fatalf("A@B 私聊消息不应入池，池内 %d 条", got)
	}

	// A@B 但正文带关键词：仍是两人私聊，关键词不得触发
	deliverAndSettle(groupMsg(group, 111, atSeg("222"), textSeg("月华你说是不是")))
	if *delivered != 0 {
		t.Fatalf("A@B 消息即使含关键词也不应触发月华")
	}
	if got := len(snapshotGroupPool(group)); got != 0 {
		t.Fatalf("A@B + 关键词不应入池，池内 %d 条", got)
	}

	// @月华：正常触发
	deliverAndSettle(groupMsg(group, 111, atSeg("999"), textSeg("月华在吗")))
	if *delivered != 1 {
		t.Fatalf("@月华的消息应送达月华，实达 %d 次", *delivered)
	}

	// @月华 且 @其他人：月华在场，正常触发
	deliverAndSettle(groupMsg(group, 111, atSeg("999"), atSeg("222"), textSeg("一起看看")))
	if *delivered != 2 {
		t.Fatalf("@月华+@B 应触发，实达 %d 次", *delivered)
	}

	// 无 @ 的普通消息：不触发但入池（保留群聊上下文）
	deliverAndSettle(groupMsg(group, 111, textSeg("大家好呀")))
	if *delivered != 2 {
		t.Fatalf("普通消息不应触发月华")
	}
	if got := len(snapshotGroupPool(group)); got != 1 {
		t.Fatalf("普通消息应入池保留上下文，池内 %d 条", got)
	}
}

// TestHandleGroupMessageReplyToSelf 钉住「引用月华的发言 = 对月华说话」：
// 引用月华消息再发言应触发；引用他人消息不触发（回落缓存池）。
func TestHandleGroupMessageReplyToSelf(t *testing.T) {
	const group = int64(7700002)
	fake, delivered := startBridgeTest(t, group)

	// 引用月华（被引用消息发送者 = 999）
	fake.quotedSender = testSelfID
	deliverAndSettle(groupMsg(group, 111, replySeg("1"), textSeg("所以答案是哪个呀")))
	if *delivered != 1 {
		t.Fatalf("引用月华消息应触发月华，实达 %d 次", *delivered)
	}

	// 引用其他成员（被引用消息发送者 = 222）
	fake.quotedSender = 222
	deliverAndSettle(groupMsg(group, 111, replySeg("2"), textSeg("你说得对")))
	if *delivered != 1 {
		t.Fatalf("引用他人消息不应触发月华")
	}
	if got := len(snapshotGroupPool(group)); got != 1 {
		t.Fatalf("引用他人消息应入池，池内 %d 条", got)
	}
}

// TestDirectAtOther 钉住顶层 @ 判定：@其他成员为真；@自己/@全体/无 @ 为假。
func TestDirectAtOther(t *testing.T) {
	selfStr := "999"
	cases := []struct {
		name     string
		segments []MessageSegment
		want     bool
	}{
		{"at他人", []MessageSegment{atSeg("222")}, true},
		{"at自己", []MessageSegment{atSeg("999")}, false},
		{"at自己与at他人", []MessageSegment{atSeg("999"), atSeg("222")}, true},
		{"at全体", []MessageSegment{atSeg("all")}, false},
		{"at零号", []MessageSegment{atSeg("0")}, false},
		{"无at", []MessageSegment{textSeg("大家好")}, false},
	}
	for _, tc := range cases {
		if got := directAtOther(tc.segments, selfStr); got != tc.want {
			t.Fatalf("%s: directAtOther = %v, want %v", tc.name, got, tc.want)
		}
	}
}
