package LunarPersonality

import (
	"strings"

	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// describeResource 取回影像资源的描述文本（视觉理解智能体）
func describeResource(handle, question string) string {
	parsed := parseHandle(handle)
	if parsed == nil {
		return ""
	}
	normalized := formatHandle(*parsed)
	cached := ""
	if entry := getResourceEntry(normalized); entry != nil {
		cached = entry.ParseText
	}
	if cached != "" && question == "" {
		return cached
	}
	if parsed.Kind == "I" || parsed.Kind == "G" {
		text, err := Vision.DescribeMedia(normalized, question)
		if err != nil {
			logger.Warn(lunarCoreModule, "[视觉理解] 影像描述失败: %v", err)
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
	text, err := Vision.DescribeVideoURL(rawURL, question)
	if err != nil {
		logger.Warn(lunarCoreModule, "[视觉理解] 视频解读失败: %v", err)
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// inspectTextResource 文本句柄探查（文献解析智能体）
func inspectTextResource(handle, request string) string {
	text, err := DocumentReader.InspectText(handle, request)
	if err != nil {
		logger.Warn(lunarCoreModule, "[文献解析] 文件探查失败: %v", err)
		return ""
	}
	return strings.TrimSpace(text)
}
