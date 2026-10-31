package knowledge

// domain_db_test.go 知识库与领域库测试
//
// 覆盖：内建表按领域路由（file_mapping → text、painter_characters → character）、
// 域库注册后 ExecuteOn 可用 / 未注册时明确报错。

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"

	_ "github.com/mattn/go-sqlite3"
)

// setupKnowledgeTest 在临时目录中隔离全部数据库路径与单例。
func setupKnowledgeTest(t *testing.T) string {
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

	_ = Close()

	t.Cleanup(func() {
		_ = Close()
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
	setupKnowledgeTest(t)
	if err := InitKnowledge(config.KnowledgeDBPath); err != nil {
		t.Fatalf("初始化知识库失败: %v", err)
	}

	cases := []struct {
		table  string
		target *domainDB
	}{
		{"file_mapping", textDB},
		{"painter_characters", characterDB},
	}
	for _, tc := range cases {
		if err := ReplaceEntries(tc.table, [][]string{{"k1", "v1"}, {"k2", "v2"}}); err != nil {
			t.Fatalf("写入 %s 失败: %v", tc.table, err)
		}
		entries, err := LoadEntries(tc.table)
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
		if ok, err := TableExists(store.db, tc.table); err != nil || ok {
			t.Errorf("%s 不应再出现在 knowledge.db (ok=%v err=%v)", tc.table, ok, err)
		}
	}

	// 未登记的表仍写通用知识库
	if err := ReplaceEntries("notes", [][]string{{"a", "b"}}); err != nil {
		t.Fatalf("写入 notes 失败: %v", err)
	}
	if ok, err := TableExists(store.db, "notes"); err != nil || !ok {
		t.Errorf("用户自建表 notes 应落在 knowledge.db (ok=%v err=%v)", ok, err)
	}
}

// TestExecuteOnDomainDB 领域库 SQL 数据源不依赖 knowledge.db 初始化
func TestExecuteOnDomainDB(t *testing.T) {
	setupKnowledgeTest(t)
	result := ExecuteOn("text", "SELECT COUNT(*) AS n FROM file_mapping", nil)
	if !result.Success {
		t.Fatalf("text 数据源查询失败: %s", result.Error)
	}
	if len(result.Results) != 1 || len(result.Results[0].Rows) != 1 {
		t.Fatalf("查询结果异常: %#v", result.Results)
	}
	if n, ok := result.Results[0].Rows[0]["n"].(int64); !ok || n != 0 {
		t.Errorf("空表计数应为 0，实际 %#v", result.Results[0].Rows[0]["n"])
	}
}

// TestExecuteOnRegisteredDomain 域库登记后 ExecuteOn 可用；未登记时明确报错
func TestExecuteOnRegisteredDomain(t *testing.T) {
	setupKnowledgeTest(t)
	archivePath := filepath.Join(t.TempDir(), "archive.db")

	// 未登记：明确报错，且不回退为 database 目录内的库文件
	result := ExecuteOn("archive", "SELECT 1", nil)
	if result.Success || !strings.Contains(result.Error, "数据库域未注册") {
		t.Fatalf("未登记域库应明确报错，实际 success=%v error=%q", result.Success, result.Error)
	}

	var archiveDB *sql.DB
	RegisterDomain("archive", func() (*sql.DB, error) {
		if archiveDB != nil {
			return archiveDB, nil
		}
		db, err := sql.Open("sqlite3", archivePath)
		if err != nil {
			return nil, err
		}
		archiveDB = db
		return db, nil
	})
	t.Cleanup(func() {
		unregisterDomain("archive")
		if archiveDB != nil {
			_ = archiveDB.Close()
		}
	})

	result = ExecuteOn("archive", "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)", nil)
	if !result.Success {
		t.Fatalf("登记后 archive 域库应可用: %s", result.Error)
	}
	result = ExecuteOn("archive", "SELECT COUNT(*) AS n FROM t", nil)
	if !result.Success || len(result.Results) != 1 || len(result.Results[0].Rows) != 1 {
		t.Fatalf("登记后查询失败: %#v", result)
	}
}
