package updatepack

// prune_test.go 发布后标签清理的选择逻辑：只统计更新包通道，最多保留最近 10 个。

import (
	"testing"

	"LunarSubsystem/LunarHarness/engine/console/channel"
)

func TestSelectStaleUpdateReleasesKeepsLatest(t *testing.T) {
	releases := make([]giteeRelease, 0, 12)
	for i := 0; i < 12; i++ {
		releases = append(releases, giteeRelease{
			ID:      int64(i + 1),
			TagName: string(rune('a' + i)),
			Body:    channel.UpdateMarker + " 更新包",
		})
	}
	stale := selectStaleUpdateReleases(releases, 10)
	if len(stale) != 2 {
		t.Fatalf("应清理 2 个超出上限的标签，实得 %d", len(stale))
	}
	if stale[0].TagName != "k" || stale[1].TagName != "l" {
		t.Fatalf("应清理最旧的两个标签，实得 %+v", stale)
	}
}

func TestSelectStaleUpdateReleasesIgnoresMarket(t *testing.T) {
	releases := []giteeRelease{
		{ID: 1, TagName: "u1", Body: channel.UpdateMarker},
		{ID: 2, TagName: "m1", Body: channel.MarketMarker},
		{ID: 3, TagName: "u2", Body: channel.UpdateMarker},
		{ID: 4, TagName: "m2", Body: channel.MarketMarker},
		{ID: 5, TagName: "u3", Body: channel.UpdateMarker},
	}
	stale := selectStaleUpdateReleases(releases, 2)
	if len(stale) != 1 || stale[0].TagName != "u3" {
		t.Fatalf("市场发布不应参与计数或被删除，实得 %+v", stale)
	}
}
