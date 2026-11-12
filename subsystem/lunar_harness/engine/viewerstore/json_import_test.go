package viewerstore

// json_import_test.go 旧版 JSON 的一次性导入、幂等与失败诊断。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// isolateViewerDir 只隔离目录，不初始化库，便于先铺历史 JSON 再触发迁移。
func isolateViewerDir(t *testing.T) string {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	Close()
	t.Cleanup(func() {
		Close()
		config.LocalDir = oldDir
	})
	dir := DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建数据目录失败: %v", err)
	}
	return dir
}

func writeLegacyFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("写入历史文件 %s 失败: %v", name, err)
	}
}

// TestImportLegacyJSONImportsOnce 首次导入保留全部键（含 reply: 历史前缀），标记后不再重导。
func TestImportLegacyJSONImportsOnce(t *testing.T) {
	dir := isolateViewerDir(t)
	writeLegacyFile(t, dir, inboxSeenJSONFile, `{"keys":{
		"msg:1": 100,
		"source:1:2:3": 200,
		"reply:1:2:4": 200
	}}`)
	writeLegacyFile(t, dir, seenVideoJSONFile, `{"videos":{"BV1test":300}}`)

	if err := Init(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	keys, err := LoadInboxSeenKeys()
	if err != nil {
		t.Fatalf("读取去重键失败: %v", err)
	}
	if len(keys) != 3 || keys["reply:1:2:4"] != 200 {
		t.Fatalf("历史键未原样导入: %+v", keys)
	}
	seen, err := LoadSeenVideosSince(0)
	if err != nil || !seen["BV1test"] {
		t.Fatalf("已看记录未导入: %+v err=%v", seen, err)
	}

	// 清空表后重新打开：标记仍然存在，旧 JSON 不得回灌。
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM inbox_seen`); err != nil {
		t.Fatalf("清空去重键失败: %v", err)
	}
	Close()
	if err := Init(); err != nil {
		t.Fatalf("重新初始化失败: %v", err)
	}
	if count, err := CountRows("inbox_seen"); err != nil || count != 0 {
		t.Fatalf("标记存在时不应重新导入，实际 %d 行 err=%v", count, err)
	}
}

// TestImportLegacyJSONMissingFile 文件缺失时写入 absent 标记，不视为失败。
func TestImportLegacyJSONMissingFile(t *testing.T) {
	isolateViewerDir(t)
	if err := Init(); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM viewer_meta WHERE key = ?`, inboxSeenImportMark).Scan(&value); err != nil {
		t.Fatalf("缺少 absent 标记: %v", err)
	}
	if value != legacyImportAbsent {
		t.Fatalf("标记值应为 %q，实际 %q", legacyImportAbsent, value)
	}
}

// TestImportLegacyJSONFailureKeepsMarkerEmpty 解析失败要带文件名与偏移，且不写标记便于重试。
func TestImportLegacyJSONFailureKeepsMarkerEmpty(t *testing.T) {
	dir := isolateViewerDir(t)
	writeLegacyFile(t, dir, inboxSeenJSONFile, `{"keys":{"msg:1":`)

	if err := Init(); err != nil {
		t.Fatalf("初始化不应因迁移失败而报错: %v", err)
	}
	db, err := openStore()
	if err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	var value string
	err = db.QueryRow(`SELECT value FROM viewer_meta WHERE key = ?`, inboxSeenImportMark).Scan(&value)
	if err == nil {
		t.Fatalf("解析失败时不应写入迁移标记，实际 %q", value)
	}

	// 修好文件后重新打开即可补导。
	writeLegacyFile(t, dir, inboxSeenJSONFile, `{"keys":{"msg:1":100}}`)
	Close()
	if err := Init(); err != nil {
		t.Fatalf("重新初始化失败: %v", err)
	}
	keys, err := LoadInboxSeenKeys()
	if err != nil || keys["msg:1"] != 100 {
		t.Fatalf("修复后应补导成功: %+v err=%v", keys, err)
	}
}

// TestMarkInboxSeenKeysPrunes TTL 清理与条数截断都在同一事务内生效。
func TestMarkInboxSeenKeysPrunes(t *testing.T) {
	isolateViewerDB(t)
	now := int64(1_000_000)
	if err := MarkInboxSeenKeys([]string{"msg:a", "msg:b"}, now, now-10, 2000); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := MarkInboxSeenKeys([]string{"msg:c"}, now+100, now+90, 2000); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	keys, err := LoadInboxSeenKeys()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(keys) != 1 || keys["msg:c"] == 0 {
		t.Fatalf("过期键应被清理，实际 %+v", keys)
	}

	if err := MarkInboxSeenKeys([]string{"msg:d"}, now+200, 0, 2); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	keys, err = LoadInboxSeenKeys()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(keys) != 2 || keys["msg:c"] == 0 || keys["msg:d"] == 0 {
		t.Fatalf("条数上限应保留最新两条，实际 %+v", keys)
	}
}

// TestRecordSeenVideosKeepsLatest 同一 BV 只留一行，时间取较新值。
func TestRecordSeenVideosKeepsLatest(t *testing.T) {
	isolateViewerDB(t)
	if err := RecordSeenVideos(map[string]int64{"BV1test": 100}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := RecordSeenVideos(map[string]int64{"BV1test": 50}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if count, err := CountRows("seen_video"); err != nil || count != 1 {
		t.Fatalf("同一 BV 不应产生多行，实际 %d err=%v", count, err)
	}
	if seen, err := LoadSeenVideosSince(60); err != nil || !seen["BV1test"] {
		t.Fatalf("应保留较新的时间 100: %+v err=%v", seen, err)
	}
}

// TestForEachJSONMapEntryErrorMentionsOffset 解析错误必须能定位到文件与字节偏移。
func TestForEachJSONMapEntryErrorMentionsOffset(t *testing.T) {
	dir := isolateViewerDir(t)
	path := filepath.Join(dir, "bad.json")
	writeLegacyFile(t, dir, "bad.json", `{"keys":{"msg:1":"oops"}}`)
	err := forEachJSONMapEntry(path, "keys", func(string, int64) error { return nil })
	if err == nil {
		t.Fatal("非整数时间戳应报错")
	}
	text := err.Error()
	if !strings.Contains(text, "bad.json") || !strings.Contains(text, "字节") {
		t.Fatalf("错误信息缺少文件名或偏移: %v", err)
	}
}
