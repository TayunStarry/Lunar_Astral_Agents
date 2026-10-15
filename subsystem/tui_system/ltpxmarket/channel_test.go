package ltpxmarket

import (
	"strings"
	"testing"

	"LunarSubsystem/TuiSystem/channel"
)

// updateReleaseFixture 更新包发布（与市场共用同一仓库）
func updateReleaseFixture() giteeRelease {
	return giteeRelease{
		ID:      33,
		TagName: "v2026.10.14-1530",
		Name:    "更新包 v2026.10.14-1530",
		Body:    "[UPDATE] 星月智能 · 资源更新包 v2026.10.14-1530\n\n**这是什么**\n...",
		Assets: []giteeAsset{{
			ID:                 1,
			Name:               "lunar_updatepack_v2026.10.14-1530.7z.001",
			BrowserDownloadURL: "https://example.com/upd",
		}},
	}
}

// marketReleaseFixture 市场包发布
func marketReleaseFixture() giteeRelease {
	return giteeRelease{
		ID:      44,
		TagName: "lunar.image-studio",
		Name:    "图像工坊",
		Body:    "[LTPX] 图像工坊\nID: lunar.image-studio\nTags: Node-LTP\n\n一个图像工具 #实用工具",
		Assets: []giteeAsset{{
			ID:                 2,
			Name:               "ltpx_lunar.image-studio.ltpx",
			BrowserDownloadURL: "https://example.com/ltpx",
			Size:               1024,
		}},
	}
}

// TestReleaseKindIsolation 市场侧通道判别：更新包发布不得被判为市场包
func TestReleaseKindIsolation(t *testing.T) {
	if got := releaseKind(marketReleaseFixture()); got != channel.Market {
		t.Fatalf("市场发布应判为 Market，实际 %v", got)
	}
	if got := releaseKind(updateReleaseFixture()); got != channel.Update {
		t.Fatalf("更新包发布应判为 Update，实际 %v", got)
	}
	// 历史更新包（隔离上线前发布，无标记但有附件前缀）
	legacy := giteeRelease{
		TagName: "v2025.08.22",
		Body:    "星月智能更新包 v2025.08.22",
		Assets:  []giteeAsset{{Name: "lunar_updatepack_v2025.08.22.7z.001"}},
	}
	if got := releaseKind(legacy); got != channel.Update {
		t.Fatalf("历史更新包应判为 Update，实际 %v", got)
	}
}

// TestParseMarketBodyRejectsForeignChannel 仅靠 [LTPX] 标记不足以防串台：
// 附件前缀为更新包时同样必须被拒（双保险），且错误信息要指出通道
func TestParseMarketBodyRejectsForeignChannel(t *testing.T) {
	// 伪装成市场格式但附件是更新包分卷：判别应以附件为准
	disguised := giteeRelease{
		TagName: "v1",
		Body:    "[LTPX] 伪装市场包\nID: v1",
		Assets:  []giteeAsset{{Name: "lunar_updatepack_v1.7z.001"}},
	}
	if got := releaseKind(disguised); got != channel.Update {
		t.Fatalf("附件证据应优先于描述标记，实际 %v", got)
	}
	if err := channelMismatchError(disguised.TagName, releaseKind(disguised)); err == nil {
		t.Fatalf("应返回通道不符错误")
	} else {
		msg := err.Error()
		for _, want := range []string{"v1", "更新包", "共用同一仓库"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("错误信息缺少 %q: %s", want, msg)
			}
		}
	}
}

// TestParseMarketBodyStillParsesMarketRelease 正常市场发布仍可解析（隔离不误伤）
func TestParseMarketBodyStillParsesMarketRelease(t *testing.T) {
	item := parseMarketBody(marketReleaseFixture())
	if item == nil {
		t.Fatalf("市场发布应可解析")
	}
	if item.Title != "图像工坊" || item.TagName != "lunar.image-studio" {
		t.Fatalf("解析结果不符: %+v", item)
	}
	if item.AssetURL == "" || item.AssetSize != 1024 {
		t.Fatalf("附件信息不符: %+v", item)
	}
	if len(item.Tags) != 1 || item.Tags[0] != "Node-LTP" {
		t.Fatalf("标签解析不符: %v", item.Tags)
	}
}
