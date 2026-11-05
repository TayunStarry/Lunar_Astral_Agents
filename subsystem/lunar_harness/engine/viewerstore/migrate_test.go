package viewerstore

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// fakeLegacySource 迁移端口的假实现：按页返回内存条目，记录删除与扫描次数。
type fakeLegacySource struct {
	entries []LegacyMemoryEntry
	deleted []string
	scans   int
	listErr error
}

// ListText 按页返回条目，总条数固定为 entries 长度；listErr 非空时模拟记忆库不可读。
func (f *fakeLegacySource) ListText(offset, limit int) ([]LegacyMemoryEntry, int, error) {
	f.scans++
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	if offset >= len(f.entries) {
		return nil, len(f.entries), nil
	}
	end := offset + limit
	if end > len(f.entries) {
		end = len(f.entries)
	}
	return f.entries[offset:end], len(f.entries), nil
}

// DeleteEntry 记录被删除的源条目 ID。
func (f *fakeLegacySource) DeleteEntry(id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

// useFakeLegacyMemory 构造假迁移源并接管落库接缝，返回带删除记录的假源。
func useFakeLegacyMemory(t *testing.T, entries []LegacyMemoryEntry, saveErr error) *fakeLegacySource {
	t.Helper()
	src := &fakeLegacySource{entries: entries}
	oldObs := legacyObservationSave
	legacyObservationSave = func(obs Observation) error {
		if saveErr != nil {
			return saveErr
		}
		return SaveObservation(obs)
	}
	t.Cleanup(func() { legacyObservationSave = oldObs })
	return src
}

// TestLegacyParserSkipsDialogueAndKeepsViewerEntries 收紧识别口径：只认 [B站视频]，
// 对话前缀（[用户: …] / [群聊: …][用户: …]）永远不算观影历史。
func TestLegacyParserSkipsDialogueAndKeepsViewerEntries(t *testing.T) {
	cases := []struct {
		content string
		kind    string
		ok      bool
	}{
		{"[B站视频][评价]正文", "评价", true},
		{"[B站视频][习惯]选片口味：游戏。", "习惯", true},
		{"[印象]《A》正文", "印象", true},
		{"[B站视频]裸正文", "印象", true},
		{"[用户: 钛宇星光阁]: 你好呀", "", false},
		{"[群聊: 星月观影群][用户: 钛宇星光阁]: 今晚看什么", "", false},
		{"别的智能体的记忆", "", false},
	}
	for _, tc := range cases {
		kind, text, ok := parseLegacyMemoryContent(tc.content)
		if ok != tc.ok {
			t.Fatalf("%q 解析结果 ok=%v，期望 %v", tc.content, ok, tc.ok)
		}
		if ok && (kind != tc.kind || text == "") {
			t.Fatalf("%q 解析为 kind=%q text=%q", tc.content, kind, text)
		}
	}

	// 对话条目即使没有来源，也不能进入迁移候选池。
	if isLegacyViewerEntry(LegacyMemoryEntry{ID: "d1", Content: "[用户: X]: 你好"}) {
		t.Fatal("对话记忆不应被识别为观影历史")
	}
	if isLegacyViewerEntry(LegacyMemoryEntry{ID: "l1", Content: "[印象]生活记忆", Source: "life"}) {
		t.Fatal("其它来源不应被识别为观影历史")
	}
	if !isLegacyViewerEntry(LegacyMemoryEntry{ID: "v1", Content: "[印象]正文", Source: memorySource}) {
		t.Fatal("来源明确的观影条目应进入候选池")
	}
}

// TestMigrationReconciliationAndIdempotency 迁移对账、全成功才删源、重复执行不重复插入。
func TestMigrationReconciliationAndIdempotency(t *testing.T) {
	isolateViewerDB(t)
	entries := []LegacyMemoryEntry{
		{ID: "o1", Source: memorySource, Timestamp: 100,
			Content: "[B站视频][印象]《A》｜https://www.bilibili.com/video/BV1aa411c7mD｜主题：游戏｜氛围：轻松｜开头不错。"},
		{ID: "o2", Source: memorySource, Timestamp: 200,
			Content: "[印象]《B》｜https://www.bilibili.com/video/BV1bb411c7mE｜主题：解说｜中段一般。"},
		{ID: "p1", Source: memorySource, Timestamp: 150, Content: "[习惯]选片口味：最近常看游戏。"},
		{ID: "s1", Source: memorySource, Timestamp: 100, Content: "[评价]评论倾向：挺喜欢。"},
		{ID: "s2", Timestamp: 300, Content: "[B站视频][评价]评论倾向：越看越上头。"},
		{ID: "m1", Source: memorySource, Timestamp: 120, Content: "[感受]这一轮看了 3 条 B 站互动"},
		{ID: "n1", Source: memorySource, Timestamp: 130, Content: "[印象]没有 BV 号的杂项"},
		{ID: "b1", Timestamp: 140, Content: "[B站视频]没有 BV 的裸正文"},
		{ID: "d1", Timestamp: 160, Content: "[用户: 钛宇星光阁]: 你好呀"},
		{ID: "l1", Source: "life", Timestamp: 170, Content: "[印象]生活记忆"},
	}
	src := useFakeLegacyMemory(t, entries, nil)

	report, err := MigrateLegacyMemories(src)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if report.Source != 8 {
		t.Fatalf("源条目数应为 8（排除对话与其它来源），实际 %d", report.Source)
	}
	if report.Imported != 5 || report.Discarded != 3 || report.Failed != 0 {
		t.Fatalf("对账不符: %+v", report)
	}
	if report.BackupPath == "" {
		t.Fatal("迁移前应导出备份")
	}
	if _, err := os.Stat(report.BackupPath); err != nil {
		t.Fatalf("备份文件不存在: %v", err)
	}
	if got := len(src.deleted); got != 8 {
		t.Fatalf("应删除 8 条源条目，实际 %d", got)
	}
	for _, id := range src.deleted {
		if id == "d1" || id == "l1" {
			t.Fatalf("不应删除非观影条目: %s", id)
		}
	}

	observations, err := LoadObservations()
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 {
		t.Fatalf("观感应入库 2 条，实际 %d", len(observations))
	}
	if value, _, ok, _ := LoadProfile(); !ok || value != "最近常看游戏。" {
		t.Fatalf("选片口味入库不符: %q", value)
	}
	if value, _, ok, _ := LoadStance(); !ok || value != "越看越上头。" {
		t.Fatalf("评论倾向应保留较新的那条: %q", value)
	}

	// 重复执行：清掉完成标记后重放，不应新增任何行。
	if err := SaveMeta(migrationMetaKey, "0"); err != nil {
		t.Fatal(err)
	}
	src = useFakeLegacyMemory(t, entries, nil)
	second, err := MigrateLegacyMemories(src)
	if err != nil {
		t.Fatalf("重复执行失败: %v", err)
	}
	if second.Source != 8 || second.Failed != 0 {
		t.Fatalf("重复执行对账不符: %+v", second)
	}
	if count, _ := CountRows("viewer_observation"); count != 2 {
		t.Fatalf("重复执行不应新增观感，实际 %d", count)
	}
	if count, _ := CountRows("viewer_profile"); count != 1 {
		t.Fatalf("选片口味应保持 1 行，实际 %d", count)
	}
	if count, _ := CountRows("viewer_stance"); count != 1 {
		t.Fatalf("评论倾向应保持 1 行，实际 %d", count)
	}
}

// TestMigrationFailureKeepsSources 导入失败时一条源条目都不删。
func TestMigrationFailureKeepsSources(t *testing.T) {
	isolateViewerDB(t)
	entries := []LegacyMemoryEntry{
		{ID: "o1", Source: memorySource, Timestamp: 100,
			Content: "[印象]《A》｜https://www.bilibili.com/video/BV1aa411c7mD｜开头不错。"},
	}
	src := useFakeLegacyMemory(t, entries, errors.New("模拟写入失败"))

	report, err := MigrateLegacyMemories(src)
	if err == nil {
		t.Fatal("导入失败应返回错误")
	}
	if report.Source != 1 || report.Failed != 1 || report.Imported != 0 {
		t.Fatalf("失败对账不符: %+v", report)
	}
	if len(src.deleted) != 0 {
		t.Fatalf("导入失败时不应删除任何源条目，实际删除 %v", src.deleted)
	}
	if count, _ := CountRows("viewer_observation"); count != 0 {
		t.Fatalf("导入失败时不应有观感入库，实际 %d", count)
	}
}

// assertNoBackup 断言迁移未落过备份文件（失败路径不应产生备份）。
func assertNoBackup(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(DataDir())
	if err != nil {
		t.Fatalf("读取观影记忆目录失败: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "viewer_memory_backup_") {
			t.Fatalf("不应写备份文件，实际存在 %s", entry.Name())
		}
	}
}

// TestMigrationUnreadableMemoryStaysPending 记忆库不可读时不得标记迁移完成。
//
// 回归用例：旧实现把 ListText 的空结果当成「没有历史条目」，记忆库未就绪
// （未初始化 / 加载失败）时会直接写入完成标记，旧观影记忆从此永久跳过。
func TestMigrationUnreadableMemoryStaysPending(t *testing.T) {
	isolateViewerDB(t)
	src := &fakeLegacySource{listErr: errors.New("模拟记忆库未就绪")}

	report, err := MigrateLegacyMemories(src)
	if MigrationDone() {
		t.Fatal("记忆库不可读时不应把迁移标记为完成")
	}
	if err == nil {
		t.Fatal("记忆库不可读应返回错误，而不是当作没有历史条目")
	}
	if !strings.Contains(err.Error(), "记忆库") {
		t.Fatalf("错误应说明记忆库未就绪/读取失败，实际: %v", err)
	}
	if len(src.deleted) != 0 {
		t.Fatalf("记忆库不可读时不应删除源条目，实际 %v", src.deleted)
	}
	assertNoBackup(t)
	if report.Source != 0 || report.Imported != 0 || report.Failed != 0 {
		t.Fatalf("不可读时不应产生迁移对账: %+v", report)
	}
}

// TestMigrationRetriesAfterReadFailure 读取失败后重试能正常完成迁移。
func TestMigrationRetriesAfterReadFailure(t *testing.T) {
	isolateViewerDB(t)
	entries := []LegacyMemoryEntry{
		{ID: "o1", Source: memorySource, Timestamp: 100,
			Content: "[印象]《A》｜https://www.bilibili.com/video/BV1aa411c7mD｜开头不错。"},
	}
	if _, err := MigrateLegacyMemories(&fakeLegacySource{listErr: errors.New("模拟记忆库未就绪")}); err == nil {
		t.Fatal("首次读取失败应返回错误")
	}

	src := useFakeLegacyMemory(t, entries, nil)
	report, err := MigrateLegacyMemories(src)
	if err != nil {
		t.Fatalf("重试应能完成迁移: %v", err)
	}
	if report.Source != 1 || report.Imported != 1 || len(src.deleted) != 1 {
		t.Fatalf("重试对账不符: %+v，删除 %v", report, src.deleted)
	}
	if !MigrationDone() {
		t.Fatal("重试成功后应标记迁移完成")
	}
}

// TestMigrationEmptyMemoryMarksDone 记忆库可读但没有条目时，与旧行为一致：直接标记完成。
func TestMigrationEmptyMemoryMarksDone(t *testing.T) {
	isolateViewerDB(t)
	report, err := MigrateLegacyMemories(&fakeLegacySource{})
	if err != nil {
		t.Fatalf("空记忆库不应报错: %v", err)
	}
	if report.Source != 0 {
		t.Fatalf("空记忆库源条目应为 0，实际 %d", report.Source)
	}
	if !MigrationDone() {
		t.Fatal("空记忆库应正常标记迁移完成")
	}
	assertNoBackup(t)
}

// TestMigrationSkipsWhenMarked 完成标记存在时不再扫描记忆库。
func TestMigrationSkipsWhenMarked(t *testing.T) {
	isolateViewerDB(t)
	if err := MarkMigrationDone(); err != nil {
		t.Fatal(err)
	}
	scanned := &fakeLegacySource{}

	if _, err := MigrateLegacyMemories(scanned); err != nil {
		t.Fatal(err)
	}
	if scanned.scans != 0 {
		t.Fatalf("已迁移时不应再扫描记忆库，实际 %d 次", scanned.scans)
	}
}

// TestMigrationReadsAllPages 超过一页（200 条）时仍完整扫描，不因分页漏条。
func TestMigrationReadsAllPages(t *testing.T) {
	isolateViewerDB(t)
	entries := make([]LegacyMemoryEntry, 0, 250)
	for i := 0; i < 250; i++ {
		entries = append(entries, LegacyMemoryEntry{
			ID: fmt.Sprintf("life-%03d", i), Source: "life", Content: "[印象]生活记忆",
		})
	}
	src := &fakeLegacySource{entries: entries}

	report, err := MigrateLegacyMemories(src)
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if report.Source != 0 || len(src.deleted) != 0 {
		t.Fatalf("非观影条目不应迁移: %+v，删除 %d 条", report, len(src.deleted))
	}
	if src.scans != 2 {
		t.Fatalf("250 条应分 2 页扫描，实际 %d 次", src.scans)
	}
}

// TestLegacyObservationParseFields 旧正文能拆出标题、链接、主题、氛围与结论。
func TestLegacyObservationParseFields(t *testing.T) {
	body := "《A》｜https://www.bilibili.com/video/BV1aa411c7mD｜主题：游戏、解说｜氛围：轻松｜开头不错。"
	obs := parseLegacyObservation(body, 100)
	if obs.BV != "BV1aa411c7mD" || obs.Title != "A" {
		t.Fatalf("BV/标题解析失败: %+v", obs)
	}
	if obs.URL != "https://www.bilibili.com/video/BV1aa411c7mD" {
		t.Fatalf("链接解析失败: %+v", obs)
	}
	if obs.Topics != "游戏、解说" || obs.Tone != "轻松" || obs.Conclusion != "开头不错。" {
		t.Fatalf("主题/氛围/结论解析失败: %+v", obs)
	}
	if !strings.Contains(body, obs.Conclusion) {
		t.Fatalf("结论应来自正文: %q", obs.Conclusion)
	}
}
