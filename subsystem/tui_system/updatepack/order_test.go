package updatepack

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"LunarSubsystem/TuiSystem/channel"
)

// stubReleases 用本地服务顶替 Gitee：返回**最旧在前**的发布列表，复现线上情况。
//
// 实测故障：仓库里同时存在 v2026.10.15-2232 与 v2026.10.15-2250（都带 [UPDATE] 标记），
// 界面与「拉取最新」拿到的却是 2232。原因：Gitee 列表接口默认 direction=asc，
// 而代码假设「响应最新在前」，于是第一条命中的就是最旧的更新包。
func stubReleases(t *testing.T, releases []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(releases)
	}))
	t.Cleanup(srv.Close)

	old := giteeAPIBase
	giteeAPIBase = srv.URL
	t.Cleanup(func() { giteeAPIBase = old })
	return srv
}

// releaseJSON 构造一条发布；asset 为空表示只有源码归档
func releaseJSON(tag, createdAt, body, asset string) map[string]any {
	assets := []map[string]any{}
	if asset != "" {
		assets = append(assets, map[string]any{
			"id": 1, "name": asset, "size": 1024,
			"browser_download_url": "https://example.invalid/" + asset,
		})
	}
	return map[string]any{
		"id": 1, "tag_name": tag, "name": tag, "body": body,
		"created_at": createdAt, "assets": assets,
	}
}

// TestPullLatestPicksNewestUpdatePack 「拉取最新」必须拿到最新的更新包发布。
// 这是用户实测报错的直接回归用例：列表最旧在前时，旧代码会返回 2232。
func TestPullLatestPicksNewestUpdatePack(t *testing.T) {
	// 顺序刻意按 Gitee 的 asc：最旧在前
	stubReleases(t, []map[string]any{
		releaseJSON("v2026.10.15-2232", "2026-10-15T22:32:10+08:00",
			"[UPDATE] 星月智能 · 资源更新包 v2026.10.15-2232", "lunar_updatepack_v2026.10.15-2232.7z.001"),
		releaseJSON("v2026.10.15-2250", "2026-10-15T22:50:33+08:00",
			"[UPDATE] 星月智能 · 资源更新包 v2026.10.15-2250", "lunar_updatepack_v2026.10.15-2250.7z.001"),
	})

	cfg := Config{GiteeOwner: "o", GiteeRepo: "r"}
	rel, err := fetchRelease(cfg, PullSpec{Kind: PullLatest})
	if err != nil {
		t.Fatalf("取最新发布失败: %v", err)
	}
	if rel.TagName != "v2026.10.15-2250" {
		t.Fatalf("取最新应得到 v2026.10.15-2250，实际 %s（把最旧当成了最新）", rel.TagName)
	}
}

// TestPullLatestSkipsMarketAndEmpty 最新的是市场发布或没有附件时，必须继续向前找更新包
func TestPullLatestSkipsMarketAndEmpty(t *testing.T) {
	stubReleases(t, []map[string]any{
		releaseJSON("v2026.10.15-2232", "2026-10-15T22:32:10+08:00",
			"[UPDATE] 星月智能 · 资源更新包 v2026.10.15-2232", "lunar_updatepack_v2026.10.15-2232.7z.001"),
		releaseJSON("v2026.10.15-2250", "2026-10-15T22:50:33+08:00",
			"[LTPX] 应用市场 · 某工具", "ltpx_sometool_1.0.0.ltpx"),
		releaseJSON("v2026.10.16-0900", "2026-10-16T09:00:00+08:00",
			"[UPDATE] 星月智能 · 资源更新包 v2026.10.16-0900", ""),
	})

	cfg := Config{GiteeOwner: "o", GiteeRepo: "r"}
	rel, err := fetchRelease(cfg, PullSpec{Kind: PullLatest})
	if err != nil {
		t.Fatalf("取最新发布失败: %v", err)
	}
	if rel.TagName != "v2026.10.15-2232" {
		t.Fatalf("应跳过更新的市场发布与无附件发布，取 v2026.10.15-2232，实际 %s", rel.TagName)
	}
}

// TestListVersionsNewestFirst 版本列表必须最新在前：
// 列表默认选中第一项，顺序错了「回车拉取」就等于拉最旧的包。
func TestListVersionsNewestFirst(t *testing.T) {
	stubReleases(t, []map[string]any{
		releaseJSON("v2026.10.15-2232", "2026-10-15T22:32:10+08:00",
			"[UPDATE] 星月智能 · 资源更新包 v2026.10.15-2232", "lunar_updatepack_v2026.10.15-2232.7z.001"),
		releaseJSON("v2026.10.15-2250", "2026-10-15T22:50:33+08:00",
			"[UPDATE] 星月智能 · 资源更新包 v2026.10.15-2250", "lunar_updatepack_v2026.10.15-2250.7z.001"),
		releaseJSON("v2026.10.15-2310", "2026-10-15T23:10:00+08:00",
			"[LTPX] 应用市场 · 某工具", "ltpx_sometool_1.0.0.ltpx"),
	})

	cfg := Config{GiteeOwner: "o", GiteeRepo: "r"}
	versions, err := ListVersions(cfg)
	if err != nil {
		t.Fatalf("列举版本失败: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("应过滤掉 LTPX 市场发布，得到 2 个更新包版本，实际 %d 个: %+v", len(versions), versions)
	}
	if versions[0].TagName != "v2026.10.15-2250" {
		t.Fatalf("列表首项应为最新版本 v2026.10.15-2250，实际 %s", versions[0].TagName)
	}
	if versions[1].TagName != "v2026.10.15-2232" {
		t.Fatalf("列表次项应为 v2026.10.15-2232，实际 %s", versions[1].TagName)
	}
}

// TestReleasesQueryAsksDescending 请求必须显式要求倒序（不依赖默认值）
func TestReleasesQueryAsksDescending(t *testing.T) {
	q := releasesQuery(Config{GiteeToken: "tok"})
	if got := q.Get("direction"); got != "desc" {
		t.Fatalf("发布列表查询应带 direction=desc，实际 %q", got)
	}
	if got := q.Get("per_page"); got != "100" {
		t.Fatalf("per_page 应为 100，实际 %q", got)
	}
	if got := q.Get("access_token"); got != "tok" {
		t.Fatalf("令牌应透传，实际 %q", got)
	}
}

// TestSortReleasesNewestFirstFallback 时间戳缺失时退化到标签自然序
func TestSortReleasesNewestFirstFallback(t *testing.T) {
	list := []giteeRelease{
		{TagName: "v2026.10.15-930"},
		{TagName: "v2026.10.15-1030"},
		{TagName: "v2026.10.14-2359"},
	}
	sortReleasesNewestFirst(list)
	want := []string{"v2026.10.15-1030", "v2026.10.15-930", "v2026.10.14-2359"}
	for i, w := range want {
		if list[i].TagName != w {
			t.Fatalf("标签兜底排序第 %d 项应为 %s，实际 %s（整体: %+v）", i, w, list[i].TagName, list)
		}
	}
	// 顺带确认比较器与通道包一致
	if !channel.Newer("", "v2026.10.15-1030", "", "v2026.10.15-930") {
		t.Fatalf("channel.Newer 标签兜底判定异常")
	}
}
