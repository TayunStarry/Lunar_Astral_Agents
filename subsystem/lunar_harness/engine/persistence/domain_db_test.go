package persistence

// domain_db_test.go 数据库分域测试
//
// 覆盖：内建表按领域路由（video_summary → media、file_mapping → text、
// painter_characters → character）、历史 knowledge.db 内建表迁移、
// 历史 archive.db 视觉缓存迁移。

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"

	_ "github.com/mattn/go-sqlite3"
)

// setupDomainTest 在临时目录中隔离全部数据库路径与单例。
func setupDomainTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	origKnowledge := config.KnowledgeDBPath
	origMedia := config.MediaDBPath
	origWeb := config.WebDBPath
	origText := config.TextDBPath
	origCharacter := config.CharacterDBPath

	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "archive.db")
	config.KnowledgeDBPath = filepath.Join(tmp, "database", "knowledge.db")
	config.MediaDBPath = filepath.Join(tmp, "database", "media.db")
	config.WebDBPath = filepath.Join(tmp, "database", "web.db")
	config.TextDBPath = filepath.Join(tmp, "database", "text.db")
	config.CharacterDBPath = filepath.Join(tmp, "database", "character.db")

	if KnowledgeDatabase != nil {
		_ = KnowledgeDatabase.Close()
	}
	KnowledgeDatabase = nil
	_ = CloseArchiveDB()
	_ = CloseDomainDBs()

	t.Cleanup(func() {
		if KnowledgeDatabase != nil {
			_ = KnowledgeDatabase.Close()
		}
		KnowledgeDatabase = nil
		_ = CloseArchiveDB()
		_ = CloseDomainDBs()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
		config.KnowledgeDBPath = origKnowledge
		config.MediaDBPath = origMedia
		config.WebDBPath = origWeb
		config.TextDBPath = origText
		config.CharacterDBPath = origCharacter
	})
	return tmp
}

// TestKnowledgeBuiltinTableRouting 内建表落各自领域库，用户表仍落 knowledge.db
func TestKnowledgeBuiltinTableRouting(t *testing.T) {
	setupDomainTest(t)
	if err := InitKnowledgeDB(config.KnowledgeDBPath); err != nil {
		t.Fatalf("初始化知识库失败: %v", err)
	}

	cases := []struct {
		table  string
		target *domainDB
	}{
		{"video_summary", mediaDatabase},
		{"file_mapping", textDatabase},
		{"painter_characters", characterDatabase},
	}
	for _, tc := range cases {
		if err := KnowledgeReplaceEntries(tc.table, [][]string{{"k1", "v1"}, {"k2", "v2"}}); err != nil {
			t.Fatalf("写入 %s 失败: %v", tc.table, err)
		}
		entries, err := KnowledgeLoadEntries(tc.table)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", tc.table, err)
		}
		if len(entries) != 2 || entries[0][0] != "k1" || entries[1][1] != "v2" {
			t.Errorf("%s 往返结果异常: %#v", tc.table, entries)
		}
		db, err := tc.target.open()
		if err != nil {
			t.Fatalf("打开 %s 目标库失败: %v", tc.table, err)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tc.table).Scan(&n); err != nil || n != 2 {
			t.Errorf("%s 应落在 %s 库（行数=%d err=%v）", tc.table, tc.target.label, n, err)
		}
		if ok, err := tableExists(KnowledgeDatabase.knowledgeDB, tc.table); err != nil || ok {
			t.Errorf("%s 不应再出现在 knowledge.db (ok=%v err=%v)", tc.table, ok, err)
		}
	}

	// 未登记的表仍写通用知识库
	if err := KnowledgeReplaceEntries("notes", [][]string{{"a", "b"}}); err != nil {
		t.Fatalf("写入 notes 失败: %v", err)
	}
	if ok, err := tableExists(KnowledgeDatabase.knowledgeDB, "notes"); err != nil || !ok {
		t.Errorf("用户自建表 notes 应落在 knowledge.db (ok=%v err=%v)", ok, err)
	}
}

// TestExecuteSQLOnDomainDB 领域库 SQL 数据源不依赖 knowledge.db 初始化
func TestExecuteSQLOnDomainDB(t *testing.T) {
	setupDomainTest(t)
	result := ExecuteSQLOn("media", "SELECT COUNT(*) AS n FROM video_summary", nil)
	if !result.Success {
		t.Fatalf("media 数据源查询失败: %s", result.Error)
	}
	if len(result.Results) != 1 || len(result.Results[0].Rows) != 1 {
		t.Fatalf("查询结果异常: %#v", result.Results)
	}
	if n, ok := result.Results[0].Rows[0]["n"].(int64); !ok || n != 0 {
		t.Errorf("空表计数应为 0，实际 %#v", result.Results[0].Rows[0]["n"])
	}
}

// TestLegacyKnowledgeTablesMigrate 历史 knowledge.db 内建表迁移到领域库并删除旧表
func TestLegacyKnowledgeTablesMigrate(t *testing.T) {
	tmp := setupDomainTest(t)
	dbPath := config.KnowledgeDBPath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("打开历史库失败: %v", err)
	}
	stmts := []string{
		`CREATE TABLE video_summary (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO video_summary (key, value) VALUES ('gif-1', '动图理解')`,
		`CREATE TABLE file_mapping (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO file_mapping (key, value) VALUES ('@D.001', '{"chunkCount":3}')`,
		`CREATE TABLE painter_characters (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO painter_characters (key, value) VALUES ('小月', '{"name":"小月"}')`,
		`CREATE TABLE web_ltp_page_cache (
			url TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', domain TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '', query_hint TEXT NOT NULL DEFAULT '', fetched_at INTEGER NOT NULL)`,
		`INSERT INTO web_ltp_page_cache (url, title, domain, summary, query_hint, fetched_at)
			VALUES ('https://example.com', '示例', 'example.com', '摘要内容', '', 1000)`,
		`CREATE TABLE notes (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO notes (key, value) VALUES ('keep', '保留')`,
	}
	for _, stmt := range stmts {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("准备历史库失败(%s): %v", stmt, err)
		}
	}
	raw.Close()

	if err := InitKnowledgeDB(dbPath); err != nil {
		t.Fatalf("初始化知识库失败: %v", err)
	}

	// 四张内建表都搬走且旧表已删除
	for _, item := range legacyKnowledgeTables {
		ok, err := tableExists(KnowledgeDatabase.knowledgeDB, item.table)
		if err != nil || ok {
			t.Errorf("旧表 %s 应已从 knowledge.db 删除 (ok=%v err=%v)", item.table, ok, err)
		}
		db, err := item.target.open()
		if err != nil {
			t.Fatalf("打开 %s 目标库失败: %v", item.target.label, err)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + item.table).Scan(&n); err != nil || n != 1 {
			t.Errorf("%s 应有 1 行迁移结果（实际 %d, err=%v）", item.table, n, err)
		}
	}
	// 用户表不受影响
	if ok, err := tableExists(KnowledgeDatabase.knowledgeDB, "notes"); err != nil || !ok {
		t.Errorf("用户表 notes 应保留 (ok=%v err=%v)", ok, err)
	}
	// 迁移前备份存在
	backups, _ := filepath.Glob(filepath.Join(tmp, "backup", "database", "knowledge.before-db-split-*.db"))
	if len(backups) == 0 {
		t.Errorf("应生成 knowledge.db 迁移前备份")
	}
}

// TestLegacyVisionCacheMigrate 历史 archive.db 视觉缓存迁入 media.db
func TestLegacyVisionCacheMigrate(t *testing.T) {
	tmp := setupDomainTest(t)
	archivePath := config.ArchiveDBPath
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	raw, err := sql.Open("sqlite3", archivePath)
	if err != nil {
		t.Fatalf("打开历史资源库失败: %v", err)
	}
	// 老版 vision_cache 没有 used_at 列，验证补列后仍能迁移
	if _, err := raw.Exec(`CREATE TABLE vision_cache (
		content_hash TEXT NOT NULL, side INTEGER NOT NULL, tag TEXT NOT NULL DEFAULT '',
		cache_path TEXT NOT NULL DEFAULT '', bytes INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (content_hash, side))`); err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO vision_cache (content_hash, side, tag, cache_path, bytes, created_at)
		VALUES ('abcd000000000001', 1080, '@I.001', 'multimedia/vision/x.jpg', 123, 1000)`); err != nil {
		t.Fatalf("写旧数据失败: %v", err)
	}
	raw.Close()

	if err := InitArchiveDB(archivePath); err != nil {
		t.Fatalf("初始化资源目录失败: %v", err)
	}
	if ok, err := tableExists(ArchiveDatabase.db, "vision_cache"); err != nil || ok {
		t.Errorf("archive.db 不应再有 vision_cache (ok=%v err=%v)", ok, err)
	}
	entry, ok := VisionCacheGet("abcd000000000001", 1080)
	if !ok || entry.Tag != "@I.001" || entry.Bytes != 123 {
		t.Errorf("迁移后的视觉缓存条目异常: %#v ok=%v", entry, ok)
	}
	backups, _ := filepath.Glob(filepath.Join(tmp, "backup", "database", "archive.before-db-split-*.db"))
	if len(backups) == 0 {
		t.Errorf("应生成 archive.db 迁移前备份")
	}
}
