package WebLTP

// cache_test.go 页面摘要缓存落库位置测试：必须写 web.db，不再占 knowledge.db。

import (
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// TestPageCacheUsesWebDB get/put 往返且数据落在 web 领域库
func TestPageCacheUsesWebDB(t *testing.T) {
	tmp := t.TempDir()
	origLocal := config.LocalDir
	origWeb := config.WebDBPath
	config.LocalDir = tmp
	config.WebDBPath = filepath.Join(tmp, "database", "web.db")
	t.Cleanup(func() {
		_ = persistence.CloseDomainDBs()
		config.LocalDir = origLocal
		config.WebDBPath = origWeb
	})

	cache, err := openPageCache(time.Hour)
	if err != nil {
		t.Fatalf("打开页面缓存失败: %v", err)
	}
	const url = "https://example.com/a"
	const summary = "这是一段足够长的页面摘要内容，用于通过缓存的最短长度校验。"
	cache.put(url, "示例页", "example.com", summary, "")
	cache.close()

	// 重新取一次（共享连接不被 close 关闭）
	cache2, err := openPageCache(time.Hour)
	if err != nil {
		t.Fatalf("重新打开页面缓存失败: %v", err)
	}
	entry, ok := cache2.get(url)
	if !ok || entry.Summary != summary || entry.Domain != "example.com" {
		t.Fatalf("缓存往返异常: %#v ok=%v", entry, ok)
	}

	db, err := persistence.EnsureWebDB()
	if err != nil {
		t.Fatalf("打开 web 库失败: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM web_ltp_page_cache WHERE url = ?`, url).Scan(&n); err != nil || n != 1 {
		t.Errorf("记录应落在 web.db（行数=%d err=%v）", n, err)
	}
}
