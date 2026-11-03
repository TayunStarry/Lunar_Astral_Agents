package Dialogue

import (
	"errors"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/agents/DocumentReader"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// init 把对话者持有的解读能力注入统一资源基座：
//
// engine/resourcehub 不依赖任何 agents 包；视觉 / 文献 / 归档 / 网页 / B 站这些
// 具体实现由对话者包在初始化时注入，供对话者与生活-创造者共用同一套取回逻辑。
func init() {
	resourcehub.SetHooks(resourcehub.Hooks{
		DescribeMedia: func(handle, ask string) string {
			return describeResource(handle, ask)
		},
		DescribeVideo: describeVideoURL,
		InspectText:   inspectTextResource,
		ReadPage: func(rawURL, ask string) (string, string, error) {
			return readPageHook(rawURL, ask)
		},
		BilibiliImpression: func(rawURL, ask string) (string, error) {
			return bilibiliImpressionHook(rawURL, ask)
		},
		IsArchiveExt: func(ext string) bool {
			return ArchiveReader.IsArchiveExt(ext) || ArchiveReader.IsGGUFExt(ext)
		},
		InspectArchive: ArchiveReader.Inspect,
		IsVideoExpired: func(err error) bool {
			return errors.Is(err, Vision.ErrVideoExpired)
		},
	})
}

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
