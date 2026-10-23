package BilibiliLife

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// setupSeenTest 把 LocalDir 指到临时目录，避免测试写真实已看记录。
func setupSeenTest(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	orig := *config.LocalDir
	*config.LocalDir = tmp
	t.Cleanup(func() { *config.LocalDir = orig })
}

// TestSeenMarkAndSnapshot 钉住「选中即记为已看、下一轮选片可据此剔除」。
func TestSeenMarkAndSnapshot(t *testing.T) {
	setupSeenTest(t)

	if got := seenSnapshot(); len(got) != 0 {
		t.Fatalf("初始不应有已看记录，实际 %v", got)
	}

	markSeen("BV1xx411c7mD")
	if !seenSnapshot()["BV1xx411c7mD"] {
		t.Fatal("标记后应出现在已看快照里")
	}
	// 落盘：重新读文件也应存在
	if _, err := os.Stat(filepath.Join(*config.LocalDir, "database", "bilibili_life", "seen.json")); err != nil {
		t.Fatalf("已看记录未落盘: %v", err)
	}

	// dropSeen 用它剔除候选
	links := []videoLink{
		{Href: "https://www.bilibili.com/video/BV1xx411c7mD", Title: "看过的"},
		{Href: "https://www.bilibili.com/video/BV1yy411c7mE", Title: "没看过的"},
	}
	out := dropSeen(links, seenSnapshot())
	if len(out) != 1 || out[0].Title != "没看过的" {
		t.Fatalf("已看视频应被剔除，实际 %v", out)
	}
}

// TestPruneSeen 钉住保留期与条数上限：超期条目删除，超上限按最久未看淘汰。
func TestPruneSeen(t *testing.T) {
	now := time.Now()
	store := seenStore{Videos: map[string]int64{
		"BVold411c7mD": now.Add(-31 * 24 * time.Hour).Unix(),
		"BVnew411c7mD": now.Unix(),
	}}
	pruneSeen(&store, now)
	if _, ok := store.Videos["BVold411c7mD"]; ok {
		t.Fatal("超过 30 天的记录应被清理")
	}
	if _, ok := store.Videos["BVnew411c7mD"]; !ok {
		t.Fatal("保留期内的记录不应被清理")
	}

	// 条数上限：多写 10 条，淘汰最旧的 10 条
	store = seenStore{Videos: map[string]int64{}}
	for i := 0; i < seenMaxEntries+10; i++ {
		store.Videos[string(rune('A'+i%26))+string(rune('a'+i/26))+string(rune('0'+i%10))] = now.Add(-time.Duration(i) * time.Minute).Unix()
	}
	pruneSeen(&store, now)
	if len(store.Videos) > seenMaxEntries {
		t.Fatalf("已看记录应不超过 %d 条，实际 %d 条", seenMaxEntries, len(store.Videos))
	}
}
