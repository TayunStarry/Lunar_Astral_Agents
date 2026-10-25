package napcat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
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
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	tmp := t.TempDir()
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		_ = file.CloseArchiveDB()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
	})

	const onePixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	handle, ok := admitInboundImage(ImageData{URL: onePixelPNG})
	if !ok || handle == "" {
		t.Fatal("data URI 转发图片入库失败，会退化成 [图片]")
	}
	if handle[0] != '@' || handle[1] != 'I' {
		t.Fatalf("转发图片应返回图片句柄，实得 %q", handle)
	}

	if _, ok := admitInboundImage(ImageData{}); ok {
		t.Fatal("空图片数据不应返回句柄")
	}
}

// TestExtractSegmentTextAdmitsImage 回复引用里的图片应换成句柄占位，
// 这样写进 messages.json 的引用摘要才是 [@I.00x] 而不是不可回引的 [图片]。
func TestExtractSegmentTextAdmitsImage(t *testing.T) {
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	tmp := t.TempDir()
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		_ = file.CloseArchiveDB()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
	})

	const onePixelPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	data, _ := json.Marshal(map[string]string{"url": onePixelPNG})
	quoted := extractSegmentText(0, 0, []MessageSegment{{Type: "image", Data: data}})
	if !strings.Contains(quoted, "[@I.") || strings.Contains(quoted, "[图片]") {
		t.Fatalf("引用图片应输出句柄占位，实际 %q", quoted)
	}

	fallback := extractSegmentText(0, 0, []MessageSegment{{Type: "image", Data: json.RawMessage(`{}`)}})
	if fallback != "图片.png" {
		t.Fatalf("图片入库失败时应退化为文件名而非占位符，实际 %q", fallback)
	}
}

// TestExtractSegmentTextAdmitsMedia 引用里的视频 / 语音 / 文件同样要换成句柄，
// 不允许退化成 [视频] / [语音] / [文件] 这类无信息占位。
func TestExtractSegmentTextAdmitsMedia(t *testing.T) {
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	tmp := t.TempDir()
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	t.Cleanup(func() {
		_ = file.CloseArchiveDB()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
	})

	videoData, _ := json.Marshal(map[string]string{
		"file": "clip.mp4",
		"url":  "data:video/mp4;base64," + base64.StdEncoding.EncodeToString([]byte("fake-mp4-bytes")),
	})
	videoText := extractSegmentText(0, 0, []MessageSegment{{Type: "video", Data: videoData}})
	if !strings.Contains(videoText, "[@V.") || !strings.Contains(videoText, "clip.mp4") {
		t.Fatalf("视频应输出句柄 + 文件名，实际 %q", videoText)
	}

	audioData, _ := json.Marshal(map[string]string{
		"file": "voice.amr",
		"url":  "data:audio/mp3;base64," + base64.StdEncoding.EncodeToString([]byte("fake-mp3-bytes")),
	})
	audioText := extractSegmentText(0, 0, []MessageSegment{{Type: "record", Data: audioData}})
	if !strings.Contains(audioText, "[@A.") || !strings.Contains(audioText, "voice.mp3") {
		t.Fatalf("语音应输出句柄 + 纠正扩展名后的文件名，实际 %q", audioText)
	}

	fileData, _ := json.Marshal(map[string]string{
		"file_name": "note.md",
		"url":       "data:text/markdown;base64," + base64.StdEncoding.EncodeToString([]byte("# hi")),
	})
	fileText := extractSegmentText(0, 0, []MessageSegment{{Type: "file", Data: fileData}})
	if !strings.Contains(fileText, "[@D.") || !strings.Contains(fileText, "note.md") {
		t.Fatalf("文件应输出句柄 + 文件名，实际 %q", fileText)
	}

	for _, text := range []string{videoText, audioText, fileText} {
		for _, placeholder := range []string{"[视频]", "[语音]", "[图片]", "[文件]"} {
			if strings.Contains(text, placeholder) {
				t.Fatalf("上下文中不应出现占位符 %s，实际 %q", placeholder, text)
			}
		}
	}
}

// TestDescribeFileKeepsName 文件句柄不可得时至少要保留文件名，而不是 [文件] 占位
func TestDescribeFileKeepsName(t *testing.T) {
	text := describeFile("report.pdf", json.RawMessage(`20480`))
	if strings.Contains(text, "[文件]") || !strings.Contains(text, "report.pdf") {
		t.Fatalf("文件描述应保留文件名，实际 %q", text)
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

// ==================== 图片双通道（image 段 / file 段）====================

// testPNG 生成用于测试的最小 PNG 字节
func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v", err)
	}
	return buffer.Bytes()
}

// TestResolveImageURLReadsLocalPath 图片段只给 path（NapCat 本地缓存）时也要能取到图片
func TestResolveImageURLReadsLocalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cached.png")
	if err := os.WriteFile(path, testPNG(t, 4, 4), 0o644); err != nil {
		t.Fatalf("写测试图片失败: %v", err)
	}
	got := resolveImageURL(ImageData{Path: path})
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("path 来源未转成 data URI: %q", got)
	}
}

// TestResolveImageURLUsesFileID 图片段只给 file_id 时，必须带 file_id 调 get_image
func TestResolveImageURLUsesFileID(t *testing.T) {
	originalPath := bridgeConfig.BridgingPath
	t.Cleanup(func() { bridgeConfig.BridgingPath = originalPath })

	payload := testPNG(t, 4, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get_image" {
			http.NotFound(w, r)
			return
		}
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		if params["file_id"] != "abc123" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "failed", "message": "缺少 file_id"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"data":   map[string]any{"base64": base64.StdEncoding.EncodeToString(payload)},
		})
	}))
	defer server.Close()
	bridgeConfig.BridgingPath = server.URL

	got := resolveImageURL(ImageData{FileID: "abc123"})
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("file_id 来源未取到图片: %q", got)
	}
}

// TestParseMessageSegmentsMFaceText 旧版 NapCat 以 mface 下发商城表情：
// 没有图片地址时必须退回摘要文案，不能把整条消息当空消息丢弃
func TestParseMessageSegmentsMFaceText(t *testing.T) {
	segments := []MessageSegment{{
		Type: "mface",
		Data: json.RawMessage(`{"emoji_package_id":1,"emoji_id":"2","key":"k","summary":"[比心]"}`),
	}}
	content, hasImages, _ := parseMessageSegments(0, segments)
	if hasImages {
		t.Fatal("无图片地址的商城表情不应被标记为图片")
	}
	text, _ := content.(string)
	if !strings.Contains(text, "比心") {
		t.Fatalf("商城表情未渲染摘要: %q", text)
	}
}

// TestParseMessageSegmentsFileChannelImage 文件通道传来的图片（只给 path）要按图片处理
func TestParseMessageSegmentsFileChannelImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(path, testPNG(t, 6, 4), 0o644); err != nil {
		t.Fatalf("写测试图片失败: %v", err)
	}
	data, err := json.Marshal(map[string]any{"file": "photo.png", "name": "photo.png", "path": path})
	if err != nil {
		t.Fatalf("序列化文件段失败: %v", err)
	}
	content, hasImages, _ := parseMessageSegments(0, []MessageSegment{{Type: "file", Data: data}})
	if !hasImages {
		t.Fatalf("文件通道图片未被识别: %#v", content)
	}
	items, ok := content.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("文件通道图片内容结构异常: %#v", content)
	}
	imageItem := items[len(items)-1]
	if imageItem["type"] != "image_url" {
		t.Fatalf("文件通道图片未生成 image_url 项: %#v", imageItem)
	}
}

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

// ==================== 随机触发自动引用 / 私聊攒批 ====================

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

// TestPickQuoteMessageIDFallbacks 单条候选直接引用；回应为空时退回最新一条；
// 无候选时不引用（避免调用嵌入服务）
func TestPickQuoteMessageIDFallbacks(t *testing.T) {
	if got := pickQuoteMessageID("回复", []QuoteCandidate{{MessageID: 7, Content: "在吗"}}); got != 7 {
		t.Fatalf("单条候选应直接引用，实得 %d", got)
	}
	if got := pickQuoteMessageID("回复", nil); got != 0 {
		t.Fatalf("无候选不应引用，实得 %d", got)
	}
	multi := []QuoteCandidate{{MessageID: 1, Content: "a"}, {MessageID: 2, Content: "b"}}
	if got := pickQuoteMessageID("  ", multi); got != 2 {
		t.Fatalf("空回应应引用最新一条，实得 %d", got)
	}
}

// TestGroupRandomTriggerPreparesQuote 随机触发时要把本批次消息带进引用候选
func TestGroupRandomTriggerPreparesQuote(t *testing.T) {
	const groupID int64 = 10086
	_, _ = startBridgeTest(t, groupID)
	one := 1.0
	bridgeConfig.BridgingGroupTriggerProbability = &one
	bridgeConfig.BridgingGroupKeywords = []string{"月华"}

	message := groupMsg(groupID, 222, textSeg("今天好热"))
	message.MessageID = 9001
	handleGroupMessage(message)

	flowMutex.Lock()
	candidates := append([]QuoteCandidate(nil), currentTarget.QuoteCandidates...)
	flowMutex.Unlock()
	if len(candidates) != 1 || candidates[0].MessageID != 9001 {
		t.Fatalf("随机触发未准备引用候选: %#v", candidates)
	}

	HandleAgentResponse("text", "是挺热的呀", false)
	NotifyAgentIdle()
}

// TestPrivateMessagesAreBatched 私聊连续消息要合并成一次推送，而不是一发即回
func TestPrivateMessagesAreBatched(t *testing.T) {
	const testUserID int64 = 30001
	originalConfig := bridgeConfig
	originalSender := SendMessageToAgent
	originalAudio := SendAudioToAgent
	t.Cleanup(func() {
		bridgeConfig = originalConfig
		SendMessageToAgent = originalSender
		SendAudioToAgent = originalAudio
		stopPrivateTimer(testUserID)
		groupMutex.Lock()
		delete(privatePools, testUserID)
		groupMutex.Unlock()
		flowMutex.Lock()
		awaitingResponse = false
		responseStarted = false
		requestQueue = nil
		flowMutex.Unlock()
	})

	one := 1.0
	window := 80
	bridgeConfig = BridgingConfig{
		BridgingUsers:                     []int64{testUserID},
		BridgingPrivateTriggerProbability: &one,
		BridgingPrivateBatchWindowMS:      &window,
	}

	deliveries := make([][]map[string]any, 0)
	SendMessageToAgent = func(messages []map[string]any) { deliveries = append(deliveries, messages) }
	SendAudioToAgent = func(urls []string) {}

	newPrivate := func(text string) NapcatMessage {
		return NapcatMessage{
			SelfID:      testSelfID,
			UserID:      testUserID,
			PostType:    "message",
			MessageType: "private",
			Sender:      Sender{Nickname: "测试用户"},
			Message:     []MessageSegment{textSeg(text)},
		}
	}
	handlePrivateMessage(newPrivate("先发一句"))
	handlePrivateMessage(newPrivate("再说一句"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(deliveries) == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if len(deliveries) != 1 {
		t.Fatalf("期望合并为 1 次推送，实得 %d 次", len(deliveries))
	}
	if len(deliveries[0]) != 2 {
		t.Fatalf("合并后的消息条数应为 2，实得 %d", len(deliveries[0]))
	}
}
