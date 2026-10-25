package BilibiliViewer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParseMsgFeedReply 钉住「回复我的」的归一化：
// source_content 是对方的新回复、target_reply_content 是我的原评论；
// business_id 是评论区类型（1/17），oid 必须取 subject_id 或 uri，不能取 business_id。
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
	if video.Type != 1 || video.Oid != "111" || video.Root != "222" || video.Parent != "333" {
		t.Fatalf("视频评论归一化错误: %+v", video)
	}
	if video.Message != "谢谢你" || video.MyContent != "我的评论" || video.Kind != "reply" {
		t.Fatalf("视频评论内容错误: %+v", video)
	}
	dynamic := items[1]
	if dynamic.Type != 17 || dynamic.Oid != "444" || dynamic.Root != "555" || dynamic.Parent != "777" {
		t.Fatalf("动态评论归一化错误: %+v", dynamic)
	}
	if dynamic.Message != "哈哈" || dynamic.MyContent != "我的动态评论" {
		t.Fatalf("动态评论内容错误: %+v", dynamic)
	}
}

// TestParseMsgFeedOpusBusinessID 钉住线上踩到的坑：
// 图片动态的 business_id=11 是「评论区类型」，不是对象 ID；
// oid 必须取 subject_id（评论 rid），不能取 business_id，也不能取 opus 展示 ID。
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
		t.Fatalf("oid 应取 subject_id（评论 rid），实际 %q", item.Oid)
	}
	if item.Root != "317943121233" || item.Parent != "317943121233" {
		t.Fatalf("顶层回复的 root/parent 应指向对方评论，实际 %q/%q", item.Root, item.Parent)
	}
}

// TestParseMsgFeedAt 钉住「@我的」：source_content 才是对方发的内容，
// 且没有 target_id 时 parent 回退到 source_id（直接回复那条 @ 评论）。
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
	if item.Message != "@月华 你觉得呢" {
		t.Fatalf("@消息内容错误: %q", item.Message)
	}
	if item.Root != "888" || item.Parent != "888" {
		t.Fatalf("root/parent 应回退到 source_id，实际 %q/%q", item.Root, item.Parent)
	}
}

// TestParseMsgFeedError 钉住未登录/接口异常不会被当成「没有未读」静默吞掉。
func TestParseMsgFeedError(t *testing.T) {
	if _, err := parseMsgFeed(`{"code":-101,"message":"账号未登录"}`, "reply"); err == nil {
		t.Fatal("code != 0 应返回错误")
	}
}

// TestFilterUnreadInbox 钉住未读筛选：过期消息、已处理消息与同批重复项都要剔除。
func TestFilterUnreadInbox(t *testing.T) {
	now := time.Unix(1700000100, 0)
	items := []inboxItem{
		{Type: 1, Oid: "1", Parent: "10", SourceID: "11", MessageID: "m1", Time: now.Unix()},
		{Type: 1, Oid: "1", Parent: "10", SourceID: "11", MessageID: "m1", Time: now.Unix()},
		{Type: 1, Oid: "1", Parent: "20", SourceID: "21", MessageID: "m2", Time: now.Unix()},
		{Type: 17, Oid: "2", Parent: "30", SourceID: "31", MessageID: "m3", Time: now.Add(-8 * 24 * time.Hour).Unix()},
	}
	seen := inboxSeenStore{Keys: map[string]int64{"reply:1:1:20": now.Unix()}}
	out := filterUnreadInbox(items, seen, now)
	if len(out) != 1 || out[0].MessageID != "m1" {
		t.Fatalf("应只留下未处理的 m1，实际 %+v", out)
	}
}

// TestInboxSeenRoundTrip 钉住去重记录能落盘并读回（下一轮据此避免重复回复）。
func TestInboxSeenRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	orig := inboxSeenFile
	inboxSeenFile = func() string { return filepath.Join(tmp, "inbox.json") }
	t.Cleanup(func() { inboxSeenFile = orig })

	item := inboxItem{Type: 1, Oid: "1", Parent: "10", SourceID: "11", MessageID: "m1"}
	markInboxSeen(item)

	raw, err := os.ReadFile(inboxSeenFile())
	if err != nil {
		t.Fatalf("去重记录未落盘: %v", err)
	}
	store := loadInboxSeen()
	if !store.has(item.keys()) {
		t.Fatalf("读回后仍有未记录键，文件内容: %s", raw)
	}
	// 已记录的消息不应再被当作未读
	if got := filterUnreadInbox([]inboxItem{item}, store, time.Now()); len(got) != 0 {
		t.Fatalf("已处理消息不应再次进入未读队列: %+v", got)
	}
}

// TestFlexIDAcceptsStringAndNumber 钉住 B 站 ID 的两种编码都能解析。
func TestFlexIDAcceptsStringAndNumber(t *testing.T) {
	var fromString flexID
	if err := fromString.UnmarshalJSON([]byte(`"1234567890123456789"`)); err != nil {
		t.Fatalf("字符串 ID 解析失败: %v", err)
	}
	if fromString.String() != "1234567890123456789" {
		t.Fatalf("字符串 ID 解析错误: %s", fromString.String())
	}
	var fromNumber flexID
	if err := fromNumber.UnmarshalJSON([]byte(`42`)); err != nil {
		t.Fatalf("数字 ID 解析失败: %v", err)
	}
	if fromNumber.String() != "42" {
		t.Fatalf("数字 ID 解析错误: %s", fromNumber.String())
	}
}
