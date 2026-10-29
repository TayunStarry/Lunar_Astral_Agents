package resource

// vision_cache_test.go 统一缓存表（图片缩影）的簿记与 LRU 淘汰测试

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// tableExists 判断 SQLite 库中是否存在某表
func tableExists(db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count)
	return count > 0, err
}

// TestVisionCacheLRUEvict 容量超限按最久未用淘汰，最近使用的保留
func TestVisionCacheLRUEvict(t *testing.T) {
	tmp := setupArchiveTest(t)

	urls := []string{
		FileReadURL("multimedia/aaaa.jpg"),
		FileReadURL("multimedia/bbbb.jpg"),
		FileReadURL("multimedia/cccc.jpg"),
	}
	for index, sourceURL := range urls {
		cacheRel := "multimedia/vision/" + []string{"aaaa", "bbbb", "cccc"}[index] + "_s1080.jpg"
		cacheAbs := filepath.Join(tmp, filepath.FromSlash(cacheRel))
		if err := os.MkdirAll(filepath.Dir(cacheAbs), 0o755); err != nil {
			t.Fatalf("准备缓存目录失败: %v", err)
		}
		if err := os.WriteFile(cacheAbs, make([]byte, 100), 0o644); err != nil {
			t.Fatalf("写入副本文件失败: %v", err)
		}
		if err := VisionCachePut(VisionCacheEntry{SourceURL: sourceURL, Side: 1080, Tag: "@I.001", CachePath: cacheRel, Bytes: 100}); err != nil {
			t.Fatalf("写入簿记失败: %v", err)
		}
	}

	VisionCacheTouch(urls[0], 1080)
	if _, err := archiveDB.db.Exec(`UPDATE cache SET used_at = 1000 WHERE source_url = ?`, NormalizeURL(urls[1])); err != nil {
		t.Fatalf("回拨 used_at 失败: %v", err)
	}
	if _, err := archiveDB.db.Exec(`UPDATE cache SET used_at = 2000 WHERE source_url = ?`, NormalizeURL(urls[2])); err != nil {
		t.Fatalf("回拨 used_at 失败: %v", err)
	}
	if _, err := archiveDB.db.Exec(`UPDATE cache SET used_at = 3000 WHERE source_url = ?`, NormalizeURL(urls[0])); err != nil {
		t.Fatalf("续期 a 失败: %v", err)
	}

	removed, freed := VisionCacheEvict(250)
	if removed != 1 || freed != 100 {
		t.Fatalf("应淘汰 1 条 / 100 字节，实际 %d 条 / %d 字节", removed, freed)
	}
	if _, ok := VisionCacheGet(urls[1], 1080); ok {
		t.Errorf("最久未用的 b 应被淘汰")
	}
	if _, ok := VisionCacheGet(urls[0], 1080); !ok {
		t.Errorf("最近续期的 a 不应被淘汰")
	}
	if _, ok := VisionCacheGet(urls[2], 1080); !ok {
		t.Errorf("次新的 c 不应被淘汰")
	}
	if _, err := os.Stat(filepath.Join(tmp, "multimedia", "vision", "bbbb_s1080.jpg")); !os.IsNotExist(err) {
		t.Errorf("被淘汰条目的磁盘副本应已删除")
	}
	if _, err := os.Stat(filepath.Join(tmp, "multimedia", "vision", "aaaa_s1080.jpg")); err != nil {
		t.Errorf("保留条目的磁盘副本不应被删除: %v", err)
	}
}

// TestVisionCacheEvictUnlimited 容量为 0（不限）时不淘汰任何条目
func TestVisionCacheEvictUnlimited(t *testing.T) {
	setupArchiveTest(t)
	if err := VisionCachePut(VisionCacheEntry{SourceURL: FileReadURL("multimedia/dddd.jpg"), Side: 1080, Tag: "@I.001", Bytes: 1000}); err != nil {
		t.Fatalf("写入簿记失败: %v", err)
	}
	removed, freed := VisionCacheEvict(0)
	if removed != 0 || freed != 0 {
		t.Errorf("容量为 0 表示不限制，不应淘汰，实际 %d 条 / %d 字节", removed, freed)
	}
}

// TestCacheLivesInHandlesDB 缓存落在统一句柄库的 cache 表，旧 vision_cache 不再存在
func TestCacheLivesInHandlesDB(t *testing.T) {
	setupArchiveTest(t)
	if err := VisionCachePut(VisionCacheEntry{SourceURL: FileReadURL("multimedia/eeee.jpg"), Side: 720, Tag: "@I.002", Bytes: 10}); err != nil {
		t.Fatalf("写入簿记失败: %v", err)
	}
	if ok, err := tableExists(archiveDB.db, "vision_cache"); err != nil || ok {
		t.Errorf("句柄库不应再有 vision_cache 表 (ok=%v err=%v)", ok, err)
	}
	if ok, err := tableExists(archiveDB.db, "cache"); err != nil || !ok {
		t.Errorf("句柄库应有统一 cache 表 (ok=%v err=%v)", ok, err)
	}
	var count int
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM cache`).Scan(&count)
	if count != 1 {
		t.Errorf("cache 表应有 1 条记录，实际 %d", count)
	}
}
