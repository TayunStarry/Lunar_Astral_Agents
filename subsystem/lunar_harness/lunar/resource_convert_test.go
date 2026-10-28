package lunar

// resource_convert_test.go 未读上下文的资源句柄化：链接必须换成 @U 句柄，绝不回显地址或 BV 号

import (
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// setupConvertTest 准备隔离的资源目录（不触发链接现场提炼，避免测试里调用模型）
func setupConvertTest(t *testing.T) {
	t.Helper()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "archive.db")
	t.Cleanup(func() {
		_ = resource.Close()
		_ = knowledge.Close()
		config.LocalDir, config.ArchiveDBPath = origLocal, origArchive
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化资源目录失败: %v", err)
	}
}

// TestConvertBilibiliURLBecomesHandle 用户消息里的 B 站地址只留句柄，BV 号不得进入上下文
func TestConvertBilibiliURLBecomesHandle(t *testing.T) {
	setupConvertTest(t)

	text := textWithResourceHandles("@钛宇-月华 https://www.bilibili.com/video/BV1o4xg6RE9m 讲述了啥？", channelOrigin, nil, "")
	if !strings.Contains(text, "@U.") {
		t.Fatalf("B 站地址应换成链接句柄，实际 %q", text)
	}
	if strings.Contains(text, "BV1o4xg6RE9m") || strings.Contains(text, "bilibili.com") {
		t.Fatalf("句柄化后不应残留地址或 BV 号，实际 %q", text)
	}
	if !strings.Contains(text, "讲述了啥") {
		t.Fatalf("原问题的文字部分应保留，实际 %q", text)
	}

	// 裸 BV 号同样只能出现句柄；同一视频复用同一句柄
	bvOnly := textWithResourceHandles("BV1o4xg6RE9m 讲述了啥？", channelOrigin, nil, "")
	if strings.Contains(bvOnly, "BV1o4xg6RE9m") {
		t.Fatalf("裸 BV 号不应回显，实际 %q", bvOnly)
	}
}

// TestUseResourceLinkWithAsk 链接句柄带 ask 取回：直接给站点与摘要，不能报「不是文本类资源」
func TestUseResourceLinkWithAsk(t *testing.T) {
	setupConvertTest(t)

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:     "https://www.bilibili.com/video/BV1o4xg6RE9m",
		Title:   "海边的日落",
		Summary: "讲的是海边日落与潮汐。",
		Origin:  channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}
	var missing []string
	text := readHandleBlock(admitted.Handle, "描述一下这个内容。", &missing)
	if strings.Contains(text, "不是文本类资源") || strings.Contains(text, "探查失败") {
		t.Fatalf("链接不应走文本探查，实际 %q", text)
	}
	if !strings.Contains(text, "海边日落与潮汐") || !strings.Contains(text, "bilibili.com") {
		t.Fatalf("应返回站点与摘要，实际 %q", text)
	}
	// 已移除「不得回显完整地址与 BV 号」的限制：链接句柄可直接带完整地址。
	if !strings.Contains(text, "BV1o4xg6RE9m") {
		t.Fatalf("链接取回应包含完整地址，实际 %q", text)
	}
}

// TestConvertPageURLBecomesHandle 普通网页地址同样句柄化（未缓存时不回显完整链接）
func TestConvertPageURLBecomesHandle(t *testing.T) {
	setupConvertTest(t)

	text := textWithResourceHandles("看看这个 https://example.com/news/2026/10/28 怎么样", channelOrigin, nil, "")
	if !strings.Contains(text, "@U.") {
		t.Fatalf("网页地址应换成链接句柄，实际 %q", text)
	}
	if strings.Contains(text, "example.com/news") {
		t.Fatalf("句柄化后不应保留原始链接，实际 %q", text)
	}
}
