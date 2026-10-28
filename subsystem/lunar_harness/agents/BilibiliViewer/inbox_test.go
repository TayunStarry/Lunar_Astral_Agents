package BilibiliViewer

import (
	"os"
	"path/filepath"
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
	items, err := parseMsgFeed(raw, "reply")
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
		{"缺少对象 ID", inboxItem{SourceID: "888"}, "", "", false},
	}
	for _, c := range cases {
		root, parent, ok := replyIDs(c.item)
		if ok != c.ok || root != c.root || parent != c.parent {
			t.Fatalf("%s: root=%q parent=%q ok=%v", c.name, root, parent, ok)
		}
	}
}

// TestReplyPageURL 钉住页面地址：优先消息中心 web URI，opus 用 URI 里的页面 ID。
func TestReplyPageURL(t *testing.T) {
	opus := inboxItem{Type: 11, Oid: "413077136", SourceID: "317943121233",
		URI: "https://www.bilibili.com/opus/1262374137649692673#reply317943121233"}
	if got := replyPageURL(opus); got != opus.URI {
		t.Fatalf("应直接使用消息中心的 web URI，实际 %q", got)
	}
	video := inboxItem{Type: 1, Oid: "111", SourceID: "333"}
	if got := replyPageURL(video); got != "https://www.bilibili.com/video/av111?comment_on=1#reply333" {
		t.Fatalf("视频页地址错误：%q", got)
	}
	dy := inboxItem{Type: 17, SourceID: "777", URI: "bilibili://dynamic/444"}
	if got := replyPageURL(dy); got != "https://www.bilibili.com/opus/444#reply777" {
		t.Fatalf("动态页地址应取 URI 中的页面 ID：%q", got)
	}
	if got := replyPageURL(inboxItem{Type: 12, Oid: "9"}); got != "" {
		t.Fatalf("暂不支持的类型应返回空，实际 %q", got)
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
	items, err := parseMsgFeed(raw, "reply")
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
	if item.Oid != "413077136" {
		t.Fatalf("oid 应取 subject_id，实际 %q", item.Oid)
	}
}

// TestParseMsgFeedAt 钉住「@我的」归类。
func TestParseMsgFeedAt(t *testing.T) {
	raw := `{"code":0,"data":{"items":[
		{"id":2001,"user":{"nickname":"小刚"},"at_time":1700000002,"item":{
			"business":"comment","business_id":1,"subject_id":999,"source_id":888,
			"source_content":"@月华 你觉得呢","uri":"bilibili://video/999"}}
	]}}`
	items, err := parseMsgFeed(raw, "at")
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
	if _, err := parseMsgFeed(`{"code":-101,"message":"账号未登录"}`, "reply"); err == nil {
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

// TestInboxSeenRoundTrip 钉住去重记录能落盘并读回。
func TestInboxSeenRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	orig := inboxSeenFile
	inboxSeenFile = func() string { return filepath.Join(tmp, "inbox.json") }
	t.Cleanup(func() { inboxSeenFile = orig })

	item := inboxItem{Type: 1, Oid: "1", SourceID: "11", MessageID: "m1"}
	markInboxSeen(item)

	raw, err := os.ReadFile(inboxSeenFile())
	if err != nil {
		t.Fatalf("去重记录未落盘: %v", err)
	}
	store := loadInboxSeen()
	if !store.has(item.keys()) {
		t.Fatalf("读回后仍有未记录键，文件内容: %s", raw)
	}
	if got := filterUnreadInbox([]inboxItem{item}, store, time.Now()); len(got) != 0 {
		t.Fatalf("已处理消息不应再次进入未读队列: %+v", got)
	}
}
