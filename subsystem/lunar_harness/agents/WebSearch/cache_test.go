package WebSearch

// cache_test.go 页面访问记录落库位置测试：必须写在统一句柄库的 URL 主表。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// TestPageCacheUsesURLMaster put/fresh 往返且数据落在 handles.db 的 url_master
func TestPageCacheUsesURLMaster(t *testing.T) {
	tmp := t.TempDir()
	origLocal := config.LocalDir
	origArchive := config.ArchiveDBPath
	config.LocalDir = tmp
	config.ArchiveDBPath = filepath.Join(tmp, "database", "handles.db")
	t.Cleanup(func() {
		_ = resource.Close()
		config.LocalDir = origLocal
		config.ArchiveDBPath = origArchive
	})

	cache, err := openPageCache(time.Hour)
	if err != nil {
		t.Fatalf("打开页面缓存失败: %v", err)
	}
	const url = "https://example.com/a"
	const summary = "这是一段足够长的页面摘要内容，用于通过缓存的最短长度校验。"
	cache.put(url, "示例页", "example.com", summary)
	cache.close()

	cache2, err := openPageCache(time.Hour)
	if err != nil {
		t.Fatalf("重新打开页面缓存失败: %v", err)
	}
	if !cache2.fresh(url) {
		t.Fatalf("近期访问判定异常: %s 应命中", url)
	}

	if _, err := os.Stat(config.ArchiveDBPath); err != nil {
		t.Errorf("记录应落在 handles.db: %v", err)
	}
	entry, ok := resource.FindByURL(url)
	if !ok || entry.Title != "示例页" || entry.ParseText != summary {
		t.Errorf("记录内容异常（title=%q text=%q ok=%v）", entry.Title, entry.ParseText, ok)
	}
}
