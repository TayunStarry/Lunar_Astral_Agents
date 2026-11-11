package thinking

import (
	"strings"

	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// 资源解读能力实现：engine/resourcehub 不依赖任何 agents 包，
// 视觉 / 文献 / 归档 / 网页 / B 站的实现由装配层（app）在启动时显式注入，
// 本包不在 init() 里改全局状态。

// ReadPage 指定网页的现场读取。
func ReadPage(rawURL, ask string) (string, string, error) {
	return readPageHook(rawURL, ask)
}

// BilibiliImpression B 站地址的直驱印象。
func BilibiliImpression(rawURL, ask string) (string, error) {
	return bilibiliImpressionHook(rawURL, ask)
}

// DescribeMedia 取回影像资源的描述文本（视觉理解智能体）
func DescribeMedia(handle, question string) string {
	parsed := parseHandle(handle)
	if parsed == nil {
		return ""
	}
	normalized := formatHandle(*parsed)
	cached := ""
	fresh := false
	if entry := getResourceEntry(normalized); entry != nil {
		cached = entry.ParseText
		fresh = resource.ParseTextFresh(*entry, resource.ParseCategoryOf(*entry))
	}
	// 只有未过期缓存才能作为失败兜底；过期内容一律不回吐
	fallback := ""
	if fresh {
		fallback = cached
	}
	if fresh && question == "" {
		return cached
	}
	if parsed.Kind == "I" || parsed.Kind == "G" {
		text, err := Vision.DescribeMedia(normalized, question)
		if err != nil {
			logger.Warn(lunarCoreModule, "[视觉理解] 影像描述失败: %v", err)
			return fallback
		}
		if desc := strings.TrimSpace(text); desc != "" {
			return desc
		}
		return fallback
	}
	return fallback
}

// DescribeVideoURL 本机视频地址（@U 背后的 /file/read/）→ 观看管线
func DescribeVideoURL(rawURL, question string) (string, error) {
	text, err := Vision.DescribeVideoURL(rawURL, question)
	if err != nil {
		logger.Warn(lunarCoreModule, "[视觉理解] 视频解读失败: %v", err)
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// InspectTextResource 文本句柄探查（文献解析智能体）
func InspectTextResource(handle, request string) string {
	text, err := DocumentReader.InspectText(handle, request)
	if err != nil {
		logger.Warn(lunarCoreModule, "[文献解析] 文件探查失败: %v", err)
		return ""
	}
	return strings.TrimSpace(text)
}
