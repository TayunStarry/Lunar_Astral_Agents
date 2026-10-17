package module

// archive_vision_test.go 视觉副本缓存表的簿记与 LRU 淘汰测试
//
// 覆盖：Put/Get 往返、命中续期（Touch 驱动 LRU 顺序）、
// 容量超限按最久未用淘汰（连磁盘副本文件一起清理）、未超限不动。

import (
	"os"
	"path/filepath"
	"testing"
)

// TestVisionCacheLRUEvict 容量超限按最久未用淘汰，最近使用的保留
func TestVisionCacheLRUEvict(t *testing.T) {
	tmp := setupArchiveTest(t)
	db := ArchiveDatabase

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
		if err := db.VisionCachePut(VisionCacheEntry{ContentHash: hash, Side: 1080, Tag: "@I.001", CachePath: cacheRel, Bytes: 100}); err != nil {
			t.Fatalf("写入簿记失败: %v", err)
		}
	}

	// 续期最早的 a（模拟刚被读取过）：此刻 LRU 顺序为 b → c → a。
	// Touch 与 Put 的 used_at 是秒级时间戳，同秒内会并列失去顺序，
	// 因此把 b / c 的 used_at 显式回拨保证确定性
	db.VisionCacheTouch("aaaa000000000001", 1080)
	if _, err := db.db.Exec(`UPDATE vision_cache SET used_at = 1000 WHERE content_hash = 'bbbb000000000002'`); err != nil {
		t.Fatalf("回拨 used_at 失败: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE vision_cache SET used_at = 2000 WHERE content_hash = 'cccc000000000003'`); err != nil {
		t.Fatalf("回拨 used_at 失败: %v", err)
	}

	// 容量 250 字节（现有 300）：必须淘汰最久未用的 b（100 字节）后回到 200 ≤ 250
	removed, freed := db.VisionCacheEvict(250)
	if removed != 1 || freed != 100 {
		t.Fatalf("应淘汰 1 条 / 100 字节，实际 %d 条 / %d 字节", removed, freed)
	}
	if _, ok := db.VisionCacheGet("bbbb000000000002", 1080); ok {
		t.Errorf("最久未用的 b 应被淘汰")
	}
	if _, ok := db.VisionCacheGet("aaaa000000000001", 1080); !ok {
		t.Errorf("最近续期的 a 不应被淘汰")
	}
	if _, ok := db.VisionCacheGet("cccc000000000003", 1080); !ok {
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
	db := ArchiveDatabase
	if err := db.VisionCachePut(VisionCacheEntry{ContentHash: "dddd000000000004", Side: 1080, Tag: "@I.001", CachePath: "", Bytes: 1000}); err != nil {
		t.Fatalf("写入簿记失败: %v", err)
	}
	removed, freed := db.VisionCacheEvict(0)
	if removed != 0 || freed != 0 {
		t.Errorf("容量为 0 表示不限制，不应淘汰，实际 %d 条 / %d 字节", removed, freed)
	}
}
