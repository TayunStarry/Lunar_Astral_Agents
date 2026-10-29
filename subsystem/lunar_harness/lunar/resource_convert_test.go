package lunar

// resource_convert_test.go 未读上下文的资源句柄化与 use_resource 分流
//
// 新体系：B 站链接 → @V 视频句柄（指向完整页面 URL）；普通网页 → @U 网页句柄；
// 已有解析文本直接返回，缺失才按类型触发解读。

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// setupConvertTest 准备隔离的统一句柄库（不触发任何模型调用）
func setupConvertTest(t *testing.T) {
	t.Helper()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	t.Cleanup(func() {
		_ = resource.Close()
		config.LocalDir, config.ArchiveDBPath = origLocal, origArchive
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化句柄库失败: %v", err)
	}
}

// TestConvertBilibiliURLBecomesHandle 用户消息里的 B 站地址只留视频句柄，BV 号不得进入上下文
func TestConvertBilibiliURLBecomesHandle(t *testing.T) {
	setupConvertTest(t)

	text := textWithResourceHandles("@钛宇-月华 https://www.bilibili.com/video/BV1o4xg6RE9m 讲述了啥？", channelOrigin, nil, "")
	if !strings.Contains(text, "@V.") {
		t.Fatalf("B 站地址应换成视频句柄，实际 %q", text)
	}
	if strings.Contains(text, "BV1o4xg6RE9m") || strings.Contains(text, "bilibili.com") {
		t.Fatalf("句柄化后不应残留地址或 BV 号，实际 %q", text)
	}
	if !strings.Contains(text, "讲述了啥") {
		t.Fatalf("原问题的文字部分应保留，实际 %q", text)
	}

	bvOnly := textWithResourceHandles("BV1o4xg6RE9m 讲述了啥？", channelOrigin, nil, "")
	if containsBV(bvOnly, "BV1o4xg6RE9m") {
		t.Fatalf("裸 BV 号不应回显，实际 %q", bvOnly)
	}
}

// containsBV 判断文本除句柄之外的正文里是否仍残留 BV 号
func containsBV(text, bvid string) bool {
	cleaned := strings.ReplaceAll(text, "@V.", "")
	return strings.Contains(cleaned, bvid)
}

// TestUseResourceLinkReturnsParsedText 已有解析文本的链接直接返回，不再触发读取
func TestUseResourceLinkReturnsParsedText(t *testing.T) {
	setupConvertTest(t)
	old := readPageHook
	readPageHook = func(string, string) (string, string, error) {
		t.Fatalf("已有解析文本时不应再次读取页面")
		return "", "", nil
	}
	t.Cleanup(func() { readPageHook = old })

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:     "https://example.com/a",
		Title:   "海边的日落",
		Summary: "讲的是海边日落与潮汐。",
		Origin:  channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}
	if admitted.Kind != resource.KindURL {
		t.Fatalf("普通网页应为 @U，实际 %s", admitted.Kind)
	}
	var missing []string
	text := readHandleBlock(admitted.Handle, "描述一下这个内容。", &missing)
	if !strings.Contains(text, "海边日落与潮汐") {
		t.Fatalf("应直接返回解析文本，实际 %q", text)
	}
}

// TestUseResourceUnwatchedBilibiliDirectImpression 未看过的 B 站视频走直驱印象并写入解析文本
func TestUseResourceUnwatchedBilibiliDirectImpression(t *testing.T) {
	setupConvertTest(t)
	old := bilibiliImpressionHook
	calls := 0
	bilibiliImpressionHook = func(string, string) (string, error) {
		calls++
		return "视频印象：讲的是本地部署模型实测，弹幕多在讨论显卡占用。", nil
	}
	t.Cleanup(func() { bilibiliImpressionHook = old })

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:    "https://www.bilibili.com/video/BV1DLpF6r6pH",
		Title:  "分享一个视频",
		Origin: channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}
	if admitted.Kind != resource.KindVideo {
		t.Fatalf("B 站链接应为 @V，实际 %s", admitted.Kind)
	}
	var missing []string
	text := readHandleBlock(admitted.Handle, "这个视频讲了啥？", &missing)
	if !strings.Contains(text, "显卡占用") {
		t.Fatalf("未看过的视频应返回视频印象，实际 %q", text)
	}
	if strings.TrimSpace(resource.ParseTextOf(admitted.URL)) == "" {
		t.Fatalf("视频印象应写入 URL 主表")
	}

	// 第二次读取直接命中解析文本，不再触发直驱
	var missingAgain []string
	again := readHandleBlock(admitted.Handle, "再说一遍", &missingAgain)
	if calls != 1 {
		t.Fatalf("已有解析文本后不应重复直驱，实际调用 %d 次", calls)
	}
	if !strings.Contains(again, "显卡占用") {
		t.Fatalf("第二次读取应命中解析文本，实际 %q", again)
	}
}

// TestUseResourceBilibiliImpressionFailure 直驱失败时只给一句说明，不冒充印象
func TestUseResourceBilibiliImpressionFailure(t *testing.T) {
	setupConvertTest(t)
	old := bilibiliImpressionHook
	bilibiliImpressionHook = func(string, string) (string, error) {
		return "", errors.New("读取视频信息失败: timeout")
	}
	t.Cleanup(func() { bilibiliImpressionHook = old })

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:    "https://www.bilibili.com/video/BV1DLpF6r6pH",
		Title:  "分享一个视频",
		Origin: channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}
	var missing []string
	text := readHandleBlock(admitted.Handle, "这个视频讲了啥？", &missing)
	if !strings.Contains(text, "视频印象获取失败") {
		t.Fatalf("直驱失败时应明确说明，实际 %q", text)
	}
}

// TestUseResourceWatchedBilibiliKeepsSummary 已有解析文本的视频直接返回，不重复直驱
func TestUseResourceWatchedBilibiliKeepsSummary(t *testing.T) {
	setupConvertTest(t)
	old := bilibiliImpressionHook
	bilibiliImpressionHook = func(string, string) (string, error) {
		t.Fatalf("已有解析文本时不应重复直驱")
		return "", nil
	}
	t.Cleanup(func() { bilibiliImpressionHook = old })

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:     "https://www.bilibili.com/video/BV1DLpF6r6pH",
		Title:   "分享一个视频",
		Summary: "讲的是本地部署模型实测。",
		Origin:  channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}
	var missing []string
	text := readHandleBlock(admitted.Handle, "这个视频讲了啥？", &missing)
	if !strings.Contains(text, "本地部署模型实测") {
		t.Fatalf("应直接返回已有解析文本，实际 %q", text)
	}
}

// TestUseResourceUnreadPageFetchesOnDemand 没读过的普通网页现场读取一次并回填解析文本
func TestUseResourceUnreadPageFetchesOnDemand(t *testing.T) {
	setupConvertTest(t)
	old := readPageHook
	calls := 0
	readPageHook = func(string, string) (string, string, error) {
		calls++
		return "示例页", "页面摘要：讲的是本地部署模型实测。", nil
	}
	t.Cleanup(func() { readPageHook = old })

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:    "https://example.com/news/2026/10/28",
		Title:  "示例页",
		Origin: channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}

	var missing []string
	text := readHandleBlock(admitted.Handle, "看看这个页面讲了什么", &missing)
	if !strings.Contains(text, "本地部署模型实测") {
		t.Fatalf("未读过的网页应现场读取并返回摘要，实际 %q", text)
	}

	var missingAgain []string
	again := readHandleBlock(admitted.Handle, "再说一遍", &missingAgain)
	if calls != 1 {
		t.Fatalf("解析文本回填后不应重复开页面，实际调用 %d 次", calls)
	}
	if !strings.Contains(again, "本地部署模型实测") {
		t.Fatalf("第二次读取应命中已回填摘要，实际 %q", again)
	}
}

// TestUseResourceUnreadPageFetchFailure 现场读取失败时只给一句说明，不冒充摘要
func TestUseResourceUnreadPageFetchFailure(t *testing.T) {
	setupConvertTest(t)
	old := readPageHook
	readPageHook = func(string, string) (string, string, error) {
		return "", "", errors.New("打开页面失败: timeout")
	}
	t.Cleanup(func() { readPageHook = old })

	admitted, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:    "https://example.com/news/2026/10/28",
		Title:  "示例页",
		Origin: channelOrigin,
	})
	if err != nil {
		t.Fatalf("链接入仓失败: %v", err)
	}

	var missing []string
	text := readHandleBlock(admitted.Handle, "看看这个页面讲了什么", &missing)
	if !strings.Contains(text, "打不开") {
		t.Fatalf("读取失败时应明确说明，实际 %q", text)
	}
}

// TestConvertPageURLBecomesHandle 普通网页地址同样句柄化
func TestConvertPageURLBecomesHandle(t *testing.T) {
	setupConvertTest(t)

	text := textWithResourceHandles("看看这个 https://example.com/news/2026/10/28 怎么样", channelOrigin, nil, "")
	if !strings.Contains(text, "@U.") {
		t.Fatalf("网页地址应换成网页句柄，实际 %q", text)
	}
	if strings.Contains(text, "example.com/news") {
		t.Fatalf("句柄化后不应保留原始链接，实际 %q", text)
	}
}

// TestContentItemTextKeyAlwaysPresent 空文本项也必须带 text 键
func TestContentItemTextKeyAlwaysPresent(t *testing.T) {
	raw, err := json.Marshal(ContentItem{Type: ContentText})
	if err != nil {
		t.Fatalf("序列化文本项失败: %v", err)
	}
	if string(raw) != `{"type":"text","text":""}` {
		t.Fatalf("空文本项应保留 text 键，实际 %s", raw)
	}

	raw, err = json.Marshal(ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: "http://example.com/a.jpg"}})
	if err != nil {
		t.Fatalf("序列化图片项失败: %v", err)
	}
	if strings.Contains(string(raw), `"text"`) {
		t.Fatalf("图片项不应带 text 键，实际 %s", raw)
	}
}
