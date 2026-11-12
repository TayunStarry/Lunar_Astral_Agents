package viewer

import (
	"testing"
	"time"
)

// TestParseMsgFeedReply 钉住「回复我的」的归一化：
// business_id 是评论区类型（1/17），oid 必须取 subject_id 或 uri。
func TestParseMsgFeedReply(t *testing.T) {
	raw := `{"code":0,"data":{"items":[
		{"id":1001,"user":{"nickname":"小明"},"reply_time":1700000000,"item":{
			"business":"comment","business_id":1,"subject_id":111,
			"root_id":0,"target_id":222,"source_id":333,
			"source_content":"谢谢你","target_reply_content":"我的评论",
			"title":"视频A","uri":"bilibili://video/111"}},
		{"id":1002,"user":{"nickname":"小红"},"reply_time":1700000001,"item":{
			"business":"dynamic","business_id":17,"subject_id":444,
			"root_id":555,"target_id":666,"source_id":777,
			"source_content":"哈哈","target_reply_content":"我的动态评论",
			"uri":"bilibili://dynamic/444"}}
	]}}`
	items, _, err := parseMsgFeedPage(raw, "reply")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应解析出 2 条，实际 %d", len(items))
	}
	video := items[0]
	if video.Type != 1 || video.Oid != "111" || video.SourceID != "333" {
		t.Fatalf("视频评论归一化错误: %+v", video)
	}
	if video.Category != notifyVideoReply {
		t.Fatalf("有目标评论应归到视频追评，实际 %v", video.Category)
	}
	if video.Message != "谢谢你" || video.MyContent != "我的评论" || video.Kind != "reply" {
		t.Fatalf("视频评论内容错误: %+v", video)
	}
	if video.TargetID != "222" || video.RootID != "" {
		t.Fatalf("视频评论应保留 target_id、root_id 为空: %+v", video)
	}
	dynamic := items[1]
	if dynamic.Type != 17 || dynamic.Oid != "444" || dynamic.SourceID != "777" {
		t.Fatalf("动态评论归一化错误: %+v", dynamic)
	}
	if dynamic.RootID != "555" || dynamic.TargetID != "666" {
		t.Fatalf("动态评论应保留 root_id/target_id: %+v", dynamic)
	}
	if dynamic.Category != notifyDynamic {
		t.Fatalf("动态互动应归到 notifyDynamic，实际 %v", dynamic.Category)
	}
}

// TestReplyIDs 钉住 root/parent 推导：子回复、我的根评论被回复、根级评论三种情况。
func TestReplyIDs(t *testing.T) {
	cases := []struct {
		name         string
		item         inboxItem
		root, parent string
		ok           bool
	}{
		{"子回复", inboxItem{Oid: "111", RootID: "555", TargetID: "666", SourceID: "777"}, "555", "777", true},
		{"我的根评论被回复", inboxItem{Oid: "111", TargetID: "222", SourceID: "333"}, "222", "333", true},
		{"根级评论直接回复", inboxItem{Oid: "444", SourceID: "888"}, "888", "888", true},
		{"@消息只有对象 ID", inboxItem{Oid: "444"}, "", "", true},
		{"缺少对象 ID", inboxItem{SourceID: "888"}, "", "", false},
	}
	for _, c := range cases {
		root, parent, ok := replyIDs(c.item)
		if ok != c.ok || root != c.root || parent != c.parent {
			t.Fatalf("%s: root=%q parent=%q ok=%v", c.name, root, parent, ok)
		}
	}
}

// TestParseMsgFeedOpusBusinessID 钉住 business_id=11 是评论区类型，不是对象 ID。
func TestParseMsgFeedOpusBusinessID(t *testing.T) {
	raw := `{"code":0,"data":{"items":[
		{"id":3001,"user":{"nickname":"钛宇-星光阁"},"reply_time":1792762157,"item":{
			"business":"dynamic","business_id":11,"subject_id":413077136,
			"root_id":0,"target_id":0,"source_id":317943121233,
			"source_content":"你为啥不看Qwen相关的视频呢？",
			"uri":"https://www.bilibili.com/opus/1262374137649692673#reply317943121233"}}
	]}}`
	items, _, err := parseMsgFeedPage(raw, "reply")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("应解析出 1 条，实际 %d", len(items))
	}
	item := items[0]
	if item.Type != 11 {
		t.Fatalf("图片动态应以 business_id=11 作为评论区类型，实际 %d", item.Type)
	}
	if item.Oid != "1262374137649692673" {
		t.Fatalf("opus 的评论区 oid 应取 URI 页面 ID，实际 %q", item.Oid)
	}
	if item.SourceID != "317943121233" {
		t.Fatalf("source_id 应保留，实际 %q", item.SourceID)
	}
}

// TestParseMsgFeedAt 钉住「@我的」归类。
func TestParseMsgFeedAt(t *testing.T) {
	raw := `{"code":0,"data":{"items":[
		{"id":2001,"user":{"nickname":"小刚"},"at_time":1700000002,"item":{
			"business":"comment","business_id":1,"subject_id":999,"source_id":888,
			"source_content":"@月华 你觉得呢","uri":"bilibili://video/999"}}
	]}}`
	items, _, err := parseMsgFeedPage(raw, "at")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("应解析出 1 条，实际 %d", len(items))
	}
	item := items[0]
	if item.Kind != "at" || item.Type != 1 || item.Oid != "999" {
		t.Fatalf("@消息归一化错误: %+v", item)
	}
	if item.Category != notifyAt {
		t.Fatalf("@消息应归到 notifyAt，实际 %v", item.Category)
	}
	if item.Message != "@月华 你觉得呢" {
		t.Fatalf("@消息内容错误: %q", item.Message)
	}
}

// TestParseMsgFeedError 钉住未登录/接口异常不会被静默吞掉。
func TestParseMsgFeedError(t *testing.T) {
	if _, _, err := parseMsgFeedPage(`{"code":-101,"message":"账号未登录"}`, "reply"); err == nil {
		t.Fatal("code != 0 应返回错误")
	}
}

// TestFilterUnreadInbox 钉住未读筛选：过期消息、已处理消息与同批重复项都要剔除。
func TestFilterUnreadInbox(t *testing.T) {
	now := time.Unix(1700000100, 0)
	items := []inboxItem{
		{Type: 1, Oid: "1", SourceID: "11", MessageID: "m1", Time: now.Unix()},
		{Type: 1, Oid: "1", SourceID: "11", MessageID: "m1", Time: now.Unix()},
		{Type: 1, Oid: "1", SourceID: "21", MessageID: "m2", Time: now.Unix()},
		{Type: 17, Oid: "2", SourceID: "31", MessageID: "m3", Time: now.Add(-8 * 24 * time.Hour).Unix()},
	}
	seen := inboxSeenStore{Keys: map[string]int64{"source:1:1:21": now.Unix()}}
	out := filterUnreadInbox(items, seen, now)
	if len(out) != 1 || out[0].MessageID != "m1" {
		t.Fatalf("应只留下未处理的 m1，实际 %+v", out)
	}
}

// TestInboxSeenRoundTrip 钉住去重记录能落库并读回。
func TestInboxSeenRoundTrip(t *testing.T) {
	isolateViewerDB(t)

	item := inboxItem{Type: 1, Oid: "1", SourceID: "11", MessageID: "m1"}
	markInboxSeen(item)

	store := loadInboxSeen()
	if !store.has(item.keys()) {
		t.Fatalf("读回后仍有未记录键: %+v", store.Keys)
	}
	if got := filterUnreadInbox([]inboxItem{item}, store, time.Now()); len(got) != 0 {
		t.Fatalf("已处理消息不应再次进入未读队列: %+v", got)
	}
}
