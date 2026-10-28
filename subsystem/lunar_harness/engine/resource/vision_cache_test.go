package resource

// vision_cache_test.go 视觉副本缓存表的簿记与 LRU 淘汰测试
//
// 覆盖：Put/Get 往返、命中续期（Touch 驱动 LRU 顺序）、
// 容量超限按最久未用淘汰（连磁盘副本文件一起清理）、未超限不动、
// 历史 archive.db 视觉缓存迁入 media.db。

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/knowledge"
)

// TestVisionCacheLRUEvict 容量超限按最久未用淘汰，最近使用的保留
func TestVisionCacheLRUEvict(t *testing.T) {
	tmp := setupArchiveTest(t)

	// 三个副本条目：各 100 字节，写入顺序 a → b → c
	hashes := []string{"aaaa000000000001", "bbbb000000000002", "cccc000000000003"}
	for _, hash := range hashes {
		cacheRel := "multimedia/vision/" + hash + "_s1080.jpg"
		cacheAbs := filepath.Join(tmp, filepath.FromSlash(cacheRel))
		if err := os.MkdirAll(filepath.Dir(cacheAbs), 0o755); err != nil {
			t.Fatalf("准备缓存目录失败: %v", err)
		}
		if err := os.WriteFile(cacheAbs, make([]byte, 100), 0o644); err != nil {
			t.Fatalf("写入副本文件失败: %v", err)
		}
		if err := VisionCachePut(VisionCacheEntry{ContentHash: hash, Side: 1080, Tag: "@I.001", CachePath: cacheRel, Bytes: 100}); err != nil {
			t.Fatalf("写入簿记失败: %v", err)
		}
	}

	// 续期最早的 a（模拟刚被读取过）：此刻 LRU 顺序为 b → c → a。
	// Touch 与 Put 的 used_at 是秒级时间戳，同秒内会并列失去顺序，
	// 因此把 b / c 的 used_at 显式回拨保证确定性
	VisionCacheTouch("aaaa000000000001", 1080)
	media, err := knowledge.Open("media")
	if err != nil {
		t.Fatalf("打开媒体派生库失败: %v", err)
	}
	if _, err := media.Exec(`UPDATE vision_cache SET used_at = 1000 WHERE content_hash = 'bbbb000000000002'`); err != nil {
		t.Fatalf("回拨 used_at 失败: %v", err)
	}
	if _, err := media.Exec(`UPDATE vision_cache SET used_at = 2000 WHERE content_hash = 'cccc000000000003'`); err != nil {
		t.Fatalf("回拨 used_at 失败: %v", err)
	}

	// 容量 250 字节（现有 300）：必须淘汰最久未用的 b（100 字节）后回到 200 ≤ 250
	removed, freed := VisionCacheEvict(250)
	if removed != 1 || freed != 100 {
		t.Fatalf("应淘汰 1 条 / 100 字节，实际 %d 条 / %d 字节", removed, freed)
	}
	if _, ok := VisionCacheGet("bbbb000000000002", 1080); ok {
		t.Errorf("最久未用的 b 应被淘汰")
	}
	if _, ok := VisionCacheGet("aaaa000000000001", 1080); !ok {
		t.Errorf("最近续期的 a 不应被淘汰")
	}
	if _, ok := VisionCacheGet("cccc000000000003", 1080); !ok {
		t.Errorf("次新的 c 不应被淘汰")
	}
	// 磁盘副本应随簿记一起被清理
	if _, err := os.Stat(filepath.Join(tmp, "multimedia", "vision", "bbbb000000000002_s1080.jpg")); !os.IsNotExist(err) {
		t.Errorf("被淘汰条目的磁盘副本应已删除")
	}
	// 未淘汰条目的副本应仍在
	if _, err := os.Stat(filepath.Join(tmp, "multimedia", "vision", "aaaa000000000001_s1080.jpg")); err != nil {
		t.Errorf("保留条目的磁盘副本不应被删除: %v", err)
	}
}

// TestVisionCacheEvictUnlimited 容量为 0（不限）时不淘汰任何条目
func TestVisionCacheEvictUnlimited(t *testing.T) {
	setupArchiveTest(t)
	if err := VisionCachePut(VisionCacheEntry{ContentHash: "dddd000000000004", Side: 1080, Tag: "@I.001", CachePath: "", Bytes: 1000}); err != nil {
		t.Fatalf("写入簿记失败: %v", err)
	}
	removed, freed := VisionCacheEvict(0)
	if removed != 0 || freed != 0 {
		t.Errorf("容量为 0 表示不限制，不应淘汰，实际 %d 条 / %d 字节", removed, freed)
	}
}

// TestVisionCacheLivesInMediaDB 视觉副本簿记落在 media.db，不再占用 archive.db
func TestVisionCacheLivesInMediaDB(t *testing.T) {
	setupArchiveTest(t)
	if err := VisionCachePut(VisionCacheEntry{ContentHash: "eeee000000000005", Side: 720, Tag: "@I.002", CachePath: "", Bytes: 10}); err != nil {
		t.Fatalf("写入簿记失败: %v", err)
	}
	if ok, err := knowledge.TableExists(archiveDB.db, "vision_cache"); err != nil || ok {
		t.Errorf("archive.db 不应再有 vision_cache 表 (ok=%v err=%v)", ok, err)
	}
	media, err := knowledge.Open("media")
	if err != nil {
		t.Fatalf("打开媒体派生库失败: %v", err)
	}
	if ok, err := knowledge.TableExists(media, "vision_cache"); err != nil || !ok {
		t.Errorf("media.db 应有 vision_cache 表 (ok=%v err=%v)", ok, err)
	}
}

// TestLegacyVisionCacheMigrate 历史 archive.db 视觉缓存迁入 media.db
func TestLegacyVisionCacheMigrate(t *testing.T) {
	tmp := setupArchiveTest(t)
	archivePath := config.ArchiveDBPath
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	// 先断开已初始化的资源目录，改造成「历史库」后重新初始化触发迁移
	_ = Close()
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

	archiveDB = nil
	archiveSeqLoaded = false
	archiveSeqCache = map[string]int64{}
	if err := Init(archivePath); err != nil {
		t.Fatalf("初始化资源目录失败: %v", err)
	}
	if ok, err := knowledge.TableExists(archiveDB.db, "vision_cache"); err != nil || ok {
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
