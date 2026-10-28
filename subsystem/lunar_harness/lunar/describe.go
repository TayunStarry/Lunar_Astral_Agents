package lunar

import (
	"strings"

	"LunarSubsystem/LunarHarness/agents/Descriptor"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// describeResource 取回资源的描述文本
func describeResource(handle, question string) string {
	parsed := parseHandle(handle)
	if parsed == nil {
		return ""
	}
	normalized := formatHandle(*parsed)
	cached := ""
	if entry := getResourceEntry(normalized); entry != nil {
		cached = entry.Summary
	}
	if cached != "" && question == "" {
		return cached
	}
	if parsed.Kind == "I" {
		text, err := Descriptor.DescribeMedia(normalized, question)
		if err != nil {
			logger.Warn(lunarCoreModule, "[描述者] 影像描述失败: %v", err)
			return cached
		}
		if desc := strings.TrimSpace(text); desc != "" {
			return desc
		}
		return cached
	}
	return cached
}

// describeVideoURL 本机视频地址（@U 背后的 /file/read/）→ 观看管线
func describeVideoURL(rawURL, question string) (string, error) {
	text, err := Descriptor.DescribeVideoURL(rawURL, question)
	if err != nil {
		logger.Warn(lunarCoreModule, "[描述者] 视频解读失败: %v", err)
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// inspectTextResource 文本句柄探查
func inspectTextResource(handle, request string) string {
	text, err := Descriptor.InspectText(handle, request)
	if err != nil {
		logger.Warn(lunarCoreModule, "[描述者] 文件探查失败: %v", err)
		return ""
	}
	return strings.TrimSpace(text)
}
