package testenv

// ==== 测试数据目录隔离 ====

import (
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// paths 一次测试内会被改写的全部本地数据路径。
type paths struct {
	LocalDir        string
	CertFile        string
	KeyFile         string
	KnowledgeDBPath string
	ArchiveDBPath   string
	TextDBPath      string
	CharacterDBPath string
	MemoryDBDir     string
}

func snapshot() paths {
	return paths{
		LocalDir:        config.LocalDir,
		CertFile:        config.CertFile,
		KeyFile:         config.KeyFile,
		KnowledgeDBPath: config.KnowledgeDBPath,
		ArchiveDBPath:   config.ArchiveDBPath,
		TextDBPath:      config.TextDBPath,
		CharacterDBPath: config.CharacterDBPath,
		MemoryDBDir:     config.MemoryDBDir,
	}
}

func apply(p paths) {
	config.LocalDir = p.LocalDir
	config.CertFile = p.CertFile
	config.KeyFile = p.KeyFile
	config.KnowledgeDBPath = p.KnowledgeDBPath
	config.ArchiveDBPath = p.ArchiveDBPath
	config.TextDBPath = p.TextDBPath
	config.CharacterDBPath = p.CharacterDBPath
	config.MemoryDBDir = p.MemoryDBDir
}

// Isolate 把 LocalDir 与全部数据库/证书路径指向同一个临时目录。
//
// 必须先关闭现有单例再改路径：resource.Init 幂等早退，不先 Close 时新路径不会生效。
// 清理阶段按相反顺序关闭临时库并还原原有路径，保证同包多个用例互不串库。
func Isolate(t testing.TB) string {
	t.Helper()

	original := snapshot()
	_ = resource.Close()
	_ = knowledge.Close()

	dir := t.TempDir()
	config.LocalDir = dir
	config.CertFile = filepath.Join(dir, "cert", "localhost.pem")
	config.KeyFile = filepath.Join(dir, "cert", "localhost-key.pem")
	config.KnowledgeDBPath = filepath.Join(dir, "database", "knowledge.db")
	config.ArchiveDBPath = filepath.Join(dir, "database", "handles.db")
	config.TextDBPath = filepath.Join(dir, "database", "text.db")
	config.CharacterDBPath = filepath.Join(dir, "database", "character.db")
	config.MemoryDBDir = filepath.Join(dir, "database", "memory")

	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化隔离句柄库失败: %v", err)
	}

	t.Cleanup(func() {
		_ = resource.Close()
		_ = knowledge.Close()
		apply(original)
	})
	return dir
}
