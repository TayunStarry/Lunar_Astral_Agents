package fileflow

// journal_migrate_test.go 旧 JSON 日志导入验收：按模式入表、可重跑、原文件保留；
// 同时确认新库首次打开不再读取旧 flow_ops.db。

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// writeLegacyJournal 写入一份旧 JSON 日志到临时 LocalDir。
func writeLegacyJournal(t *testing.T, pathRel, content string) {
	t.Helper()
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(pathRel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("创建旧日志目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("写入旧日志失败: %v", err)
	}
}

// journalReport 按模式取出导入报告。
func journalReport(t *testing.T, report JournalMigrationReport, mode Mode) JournalFileReport {
	t.Helper()
	for _, item := range report.Files {
		if item.Mode == mode {
			return item
		}
	}
	t.Fatalf("报告中缺少模式 %s", mode)
	return JournalFileReport{}
}

// TestLegacyJournalMigrationIsIdempotent 旧 JSON 日志按模式入表，重跑不重复写入。
func TestLegacyJournalMigrationIsIdempotent(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })

	writeLegacyJournal(t, legacyJournalFile, `{
		"version": 1,
		"mode": "creator",
		"last_plans": [
			{"agent": "researcher", "task": "调研"},
			{"agent": "writer", "task": "写作"}
		],
		"memos": ["备忘一", "备忘二"],
		"references": ["参考一", "参考二"],
		"errors": ["错误一", "错误二"],
		"last_summary": "本轮完成 1 步",
		"updated_at": 1793715392
	}`)
	writeLegacyJournal(t, legacyAssignedJournalFile, `{
		"version": 1,
		"mode": "assigned",
		"last_plans": [{"agent": "runner", "task": "执行指派"}],
		"memos": ["不应入表"],
		"references": ["不应入表"],
		"errors": ["指派错误"],
		"last_summary": "指派收尾",
		"updated_at": 1793715393
	}`)

	first, err := MigrateLegacyJournals()
	if err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	creator := journalReport(t, first, ModeCreator)
	if !creator.Exists || !creator.Skipped {
		t.Fatalf("自由创作日志导入后应存在且标记已导入: %+v", creator)
	}
	if creator.Steps != 2 || creator.References != 2 || creator.Memos != 2 || creator.Errors != 2 {
		t.Fatalf("自由创作行数应为 2/2/2/2: %+v", creator)
	}
	assigned := journalReport(t, first, ModeAssigned)
	if !assigned.Exists || !assigned.Skipped {
		t.Fatalf("指派日志导入后应存在且标记已导入: %+v", assigned)
	}
	if assigned.Steps != 1 || assigned.Errors != 1 || assigned.References != 0 || assigned.Memos != 0 {
		t.Fatalf("指派模式只应入步骤 / 错误: %+v", assigned)
	}

	second, err := MigrateLegacyJournals()
	if err != nil {
		t.Fatalf("重跑导入失败: %v", err)
	}
	creatorAgain := journalReport(t, second, ModeCreator)
	if creatorAgain.Steps != 2 || creatorAgain.References != 2 || creatorAgain.Memos != 2 || creatorAgain.Errors != 2 {
		t.Fatalf("重跑后自由创作行数不应变化: %+v", creatorAgain)
	}
	if assignedAgain := journalReport(t, second, ModeAssigned); assignedAgain.Steps != 1 || assignedAgain.Errors != 1 {
		t.Fatalf("重跑后指派行数不应变化: %+v", assignedAgain)
	}

	db, err := deltaConn()
	if err != nil {
		t.Fatalf("打开统一库失败: %v", err)
	}
	if got := countRows(t, db, callTable); got != 2 {
		t.Fatalf("应只有两条合成调用，实际 %d", got)
	}
	for _, rel := range []string{legacyJournalFile, legacyAssignedJournalFile} {
		if _, statErr := os.Stat(filepath.Join(config.LocalDir, filepath.FromSlash(rel))); statErr != nil {
			t.Fatalf("原 JSON 日志应保留 %s: %v", rel, statErr)
		}
	}
}

// TestNewStoreDoesNotReadLegacyFlowOps 新库首次打开不再读取旧 flow_ops.db。
func TestNewStoreDoesNotReadLegacyFlowOps(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })

	legacyPath := filepath.Join(config.LocalDir, "database", "fileflow", "flow_ops.db")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("创建旧库目录失败: %v", err)
	}
	legacy, err := sql.Open("sqlite3", legacyPath)
	if err != nil {
		t.Fatalf("创建旧库失败: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE flow_ops (id INTEGER PRIMARY KEY, flow_id TEXT, seq INTEGER, op TEXT)`); err != nil {
		legacy.Close()
		t.Fatalf("建旧表失败: %v", err)
	}
	if _, err := legacy.Exec(`INSERT INTO flow_ops (id, flow_id, seq, op) VALUES (1,'legacy-flow',1,'create')`); err != nil {
		legacy.Close()
		t.Fatalf("写入旧操作失败: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("关闭旧库失败: %v", err)
	}

	db, err := deltaConn()
	if err != nil {
		t.Fatalf("打开统一库失败: %v", err)
	}
	if got := countRows(t, db, deltaOpTable); got != 0 {
		t.Fatalf("新库不应导入旧 flow_ops 记录，实际 %d 行", got)
	}
	if value, ok, err := readMeta(db, "legacy_flow_ops_migrated"); err != nil {
		t.Fatalf("读取元数据失败: %v", err)
	} else if ok && value != "" {
		t.Fatalf("不应存在旧 flow_ops 迁移标记")
	}
}
