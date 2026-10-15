package ltpxmarket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// marketFixture 构造一条市场发布（正文格式与发布端一致）
func marketFixture(tag, id, createdAt string) giteeRelease {
	return giteeRelease{
		ID:        1,
		TagName:   tag,
		Name:      tag,
		CreatedAt: createdAt,
		Body:      "[LTPX] " + tag + "\nID: " + id + "\nTags: Node-LTP\n\n说明文本",
		Assets: []giteeAsset{{
			ID:                 2,
			Name:               "ltpx_" + id + ".ltpx",
			BrowserDownloadURL: "https://example.invalid/" + id,
			Size:               1024,
		}},
	}
}

// stubMarketAPI 用本地服务顶替 Gitee（返回顺序由调用方给定）
func stubMarketAPI(t *testing.T, releases []giteeRelease) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(releases)
	}))
	t.Cleanup(srv.Close)
	old := giteeAPIBase
	giteeAPIBase = srv.URL
	t.Cleanup(func() { giteeAPIBase = old })
}

// TestListPackagesNewestFirstWithLimit 列表必须最新在前，且 limit 截断的是"最新的 N 个"。
//
// 实测同类故障（更新包侧）已证实：Gitee 列表接口默认 asc（最旧在前）。市场侧此前既依赖
// 响应顺序又用 limit 截断，等于只列最旧的 N 个包，新发布的包永远进不了列表。
func TestListPackagesNewestFirstWithLimit(t *testing.T) {
	// 顺序刻意按 Gitee 的 asc：最旧在前
	stubMarketAPI(t, []giteeRelease{
		marketFixture("lunar.alpha", "lunar.alpha", "2026-10-15T10:00:00+08:00"),
		marketFixture("lunar.beta", "lunar.beta", "2026-10-15T11:00:00+08:00"),
		marketFixture("lunar.gamma", "lunar.gamma", "2026-10-15T12:00:00+08:00"),
	})

	cfg := Config{GiteeOwner: "o", GiteeRepo: "r"}
	items, err := ListPackages(cfg, "", 2)
	if err != nil {
		t.Fatalf("列举市场包失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("limit=2 应返回 2 项，实际 %d 项: %+v", len(items), items)
	}
	if items[0].TagName != "lunar.gamma" || items[1].TagName != "lunar.beta" {
		t.Fatalf("应返回最新的两个包 [lunar.gamma lunar.beta]，实际 [%s %s]（截断到了最旧的包）",
			items[0].TagName, items[1].TagName)
	}
}

// TestListPackagesNewestFirstNoLimit 不设 limit 时同样最新在前
func TestListPackagesNewestFirstNoLimit(t *testing.T) {
	stubMarketAPI(t, []giteeRelease{
		marketFixture("lunar.alpha", "lunar.alpha", "2026-10-15T10:00:00+08:00"),
		marketFixture("lunar.gamma", "lunar.gamma", "2026-10-15T12:00:00+08:00"),
	})
	cfg := Config{GiteeOwner: "o", GiteeRepo: "r"}
	items, err := ListPackages(cfg, "", 0)
	if err != nil {
		t.Fatalf("列举市场包失败: %v", err)
	}
	if len(items) != 2 || items[0].TagName != "lunar.gamma" {
		t.Fatalf("应最新在前，实际 %+v", items)
	}
}

// TestListPackagesRequestsDescending 请求必须显式要求倒序
func TestListPackagesRequestsDescending(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]giteeRelease{})
	}))
	t.Cleanup(srv.Close)
	old := giteeAPIBase
	giteeAPIBase = srv.URL
	t.Cleanup(func() { giteeAPIBase = old })

	if _, err := ListPackages(Config{GiteeOwner: "o", GiteeRepo: "r"}, "", 5); err != nil {
		t.Fatalf("列举失败: %v", err)
	}
	if !contains(gotQuery, "direction=desc") {
		t.Fatalf("请求应带 direction=desc，实际 query=%q", gotQuery)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
