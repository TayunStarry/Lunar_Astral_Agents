package library

// store_test.go 变更库持久化、增量、分页、去重与旧数据迁移的回归测试。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// withTempLibrary 把 LocalDir 指向临时目录，并在测试结束时关闭连接。
func withTempLibrary(t *testing.T) {
	t.Helper()
	old := config.LocalDir
	config.LocalDir = t.TempDir()
	_ = Close()
	t.Cleanup(func() {
		_ = Close()
		config.LocalDir = old
	})
}

// mustPublish 写入一条必须被记录的操作。
func mustPublish(t *testing.T, notice EditNotice) {
	t.Helper()
	PublishEdit(notice)
}

// TestPublishPersistsAcrossRestart 关闭并重新打开连接后，记录与版本仍在。
func TestPublishPersistsAcrossRestart(t *testing.T) {
	withTempLibrary(t)
	mustPublish(t, EditNotice{
		Path: "documents/a.md", Op: "write", Added: "hello", StartLine: 1, Lines: 3,
		Handle: "handle-a", At: time.Now(),
	})
	first := Snapshot()
	if first.Version != 1 || len(first.Files) != 1 {
		t.Fatalf("首次写入异常: version=%d files=%d", first.Version, len(first.Files))
	}
	if len(first.Files[0].Records) != 1 || first.Files[0].Records[0].Added != "hello" {
		t.Fatalf("记录内容异常: %+v", first.Files[0].Records)
	}
	if first.Files[0].Status != StatusActive {
		t.Fatalf("状态应为 active，实际 %s", first.Files[0].Status)
	}

	// 模拟进程重启：关闭连接后重新读取同一个库。
	if err := Close(); err != nil {
		t.Fatalf("关闭变更库失败: %v", err)
	}
	second := Snapshot()
	if second.Version != 1 || len(second.Files) != 1 {
		t.Fatalf("重启后记录丢失: version=%d files=%d", second.Version, len(second.Files))
	}
	if len(second.Files[0].Records) != 1 || second.Files[0].Records[0].Added != "hello" {
		t.Fatalf("重启后记录内容异常: %+v", second.Files[0].Records)
	}
}

// TestEditsIncrementalAndPagination 验证 since 增量、offset/limit 分页与库重置标记。
func TestEditsIncrementalAndPagination(t *testing.T) {
	withTempLibrary(t)
	mustPublish(t, EditNotice{Path: "documents/a.md", Op: "write", Added: "a", At: time.Now()})
	firstVersion := Snapshot().Version
	mustPublish(t, EditNotice{Path: "documents/b.md", Op: "write", Added: "b", At: time.Now().Add(time.Second)})

	incremental, err := Edits(EditQuery{Since: firstVersion})
	if err != nil {
		t.Fatalf("增量读取失败: %v", err)
	}
	if len(incremental.Files) != 1 || incremental.Files[0].Path != "documents/b.md" {
		t.Fatalf("增量结果异常: %+v", incremental.Files)
	}
	if incremental.Version != firstVersion+1 || incremental.Reset {
		t.Fatalf("增量版本异常: version=%d reset=%v", incremental.Version, incremental.Reset)
	}

	page, err := Edits(EditQuery{Offset: 0, Limit: 1})
	if err != nil {
		t.Fatalf("分页读取失败: %v", err)
	}
	if page.Total != 2 || !page.HasMore || len(page.Files) != 1 {
		t.Fatalf("第一页异常: total=%d hasMore=%v files=%d", page.Total, page.HasMore, len(page.Files))
	}
	next, err := Edits(EditQuery{Offset: page.Offset + page.Limit, Limit: 1})
	if err != nil {
		t.Fatalf("第二页读取失败: %v", err)
	}
	if next.HasMore || len(next.Files) != 1 {
		t.Fatalf("第二页异常: hasMore=%v files=%d", next.HasMore, len(next.Files))
	}

	Clear()
	reset, err := Edits(EditQuery{Since: firstVersion})
	if err != nil {
		t.Fatalf("重置后读取失败: %v", err)
	}
	if !reset.Reset || reset.Version != 0 || len(reset.Files) != 0 {
		t.Fatalf("重置标记异常: reset=%v version=%d files=%d", reset.Reset, reset.Version, len(reset.Files))
	}
}

// TestHistoryPagination 单文件历史按 before 游标分页，最新在前。
func TestHistoryPagination(t *testing.T) {
	withTempLibrary(t)
	base := time.Now()
	for i := 1; i <= 5; i++ {
		mustPublish(t, EditNotice{
			Path: "documents/b.md", Op: "write", Added: fmt.Sprintf("line%d", i),
			StartLine: i, Lines: 1, At: base.Add(time.Duration(i) * time.Second),
		})
	}
	first, err := History(HistoryQuery{Path: "documents/b.md", Limit: 2})
	if err != nil {
		t.Fatalf("历史第一页失败: %v", err)
	}
	if len(first.Records) != 2 || first.Records[0].Added != "line5" || first.Records[1].Added != "line4" {
		t.Fatalf("历史第一页内容异常: %+v", first.Records)
	}
	if first.Total != 5 || !first.HasMore || first.NextBefore == 0 {
		t.Fatalf("历史第一页元数据异常: %+v", first)
	}
	second, err := History(HistoryQuery{Path: "documents/b.md", Before: first.NextBefore, Limit: 2})
	if err != nil {
		t.Fatalf("历史第二页失败: %v", err)
	}
	if len(second.Records) != 2 || second.Records[0].Added != "line3" || !second.HasMore {
		t.Fatalf("历史第二页内容异常: %+v", second)
	}
	third, err := History(HistoryQuery{Path: "documents/b.md", Before: second.NextBefore, Limit: 2})
	if err != nil {
		t.Fatalf("历史第三页失败: %v", err)
	}
	if len(third.Records) != 1 || third.Records[0].Added != "line1" || third.HasMore {
		t.Fatalf("历史第三页内容异常: %+v", third)
	}
}

// TestCoalesceRepeatedRecords 窗口内重复的无正文操作合并，窗口外重新记录。
func TestCoalesceRepeatedRecords(t *testing.T) {
	withTempLibrary(t)
	base := time.Now()
	mustPublish(t, EditNotice{Path: "documents/c.md", Op: "read", At: base})
	mustPublish(t, EditNotice{Path: "documents/c.md", Op: "read", At: base.Add(2 * time.Second)})
	snapshot := Snapshot()
	if len(snapshot.Files) != 1 || len(snapshot.Files[0].Records) != 1 {
		t.Fatalf("窗口内重复操作应合并为 1 条: %+v", snapshot.Files)
	}
	if snapshot.Version != 2 {
		t.Fatalf("合并后版本仍应递增，实际 %d", snapshot.Version)
	}
	if snapshot.Files[0].Records[0].At != base.Add(2*time.Second).Unix() {
		t.Fatalf("合并后时间未刷新: %+v", snapshot.Files[0].Records[0])
	}

	mustPublish(t, EditNotice{Path: "documents/c.md", Op: "read", At: base.Add(10 * time.Minute)})
	snapshot = Snapshot()
	if len(snapshot.Files[0].Records) != 2 {
		t.Fatalf("窗口外操作应新增记录: %+v", snapshot.Files[0].Records)
	}
}

// TestMigrateLegacyJSON 旧格式编辑状态 JSON 首次打开时导入并改名。
func TestMigrateLegacyJSON(t *testing.T) {
	withTempLibrary(t)
	legacy := EditSnapshot{
		Version: 9,
		Files: []FileState{{
			Path: "documents/legacy.md", Status: StatusActive, Op: "write", At: time.Now().Unix(),
			Records: []EditRecord{{Op: "write", Added: "old", StartLine: 1, Lines: 1, At: time.Now().Unix()}},
		}},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("构造旧数据失败: %v", err)
	}
	legacyPath := filepath.Join(config.LocalDir, filepath.FromSlash(legacyEditsFileRel))
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("创建旧数据目录失败: %v", err)
	}
	if err := os.WriteFile(legacyPath, raw, 0o644); err != nil {
		t.Fatalf("写入旧数据失败: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("初始化变更库失败: %v", err)
	}
	snapshot := Snapshot()
	if len(snapshot.Files) != 1 || len(snapshot.Files[0].Records) != 1 {
		t.Fatalf("旧数据未导入: %+v", snapshot.Files)
	}
	if snapshot.Files[0].Records[0].Added != "old" || snapshot.Version != 1 {
		t.Fatalf("旧数据内容异常: %+v version=%d", snapshot.Files[0].Records, snapshot.Version)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("旧文件应已改名，实际 stat err=%v", err)
	}
}
