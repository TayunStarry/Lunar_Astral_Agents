package lunar

// resource_convert.go 未读上下文的统一资源预处理（ResourcesConversionHandle）
//
// 规范要求：
//   - 图片 → 视觉缩影链接（句柄藏在 tag 参数里）；
//   - 视频 / 动图 / 音频 → 句柄文本，绝不以裸地址或占位符进入上下文；
//   - 文本中的 URL → 句柄文本；接入阶段只登记，解析留到 use_resource；
//   - 文件 / 代码 / 未知类型 → @D 文件句柄。
//
// 任何分支都不产出「[图片]」「[视频]」这类类型占位符：拿不到句柄时退化为文件名。

import (
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
)

var (
	// 文本中的网络地址（到空白或常见中英文标点为止）
	textURLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"'）】」，。；！？、]+`)
	// 文本中裸写的 BV 号
	textBVPattern = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)
	// 地址尾部可能粘上的英文标点
	urlTrailingPunct = ".,;:!?)]}>'\""
)

// ResourcesConversionHandle 未读上下文的统一资源预处理：返回「可直接进模型上下文」的消息副本。
func ResourcesConversionHandle(messages []PostMessage) []PostMessage {
	if len(messages) == 0 {
		return messages
	}
	converted := make([]PostMessage, len(messages))
	for index := range messages {
		converted[index] = messages[index]
		if text, ok := messages[index].Content.(string); ok {
			// 工具回传的文本（检索报告等）保持原样
			if messages[index].Role == RoleTool {
				continue
			}
			converted[index].Content = textWithResourceHandles(text, channelOrigin, nil, "")
			continue
		}
		items := asItems(messages[index].Content)
		if len(items) == 0 {
			continue
		}
		out := make([]ContentItem, 0, len(items))
		for _, item := range items {
			out = append(out, convertResourceItem(item, channelOrigin, nil, ""))
		}
		converted[index].Content = out
	}
	// 直出模式下，本批语音已全部转写：统一卸载 ASR，避免逐条加载/卸载抖动
	if audioDirectTextEnabled && batchHasAudio(messages) {
		unloadASRModel()
	}
	return converted
}

// batchHasAudio 本批未读上下文是否包含音频资源
func batchHasAudio(messages []PostMessage) bool {
	for _, message := range messages {
		for _, item := range asItems(message.Content) {
			if item.Type == ContentAudioURL || item.Type == ContentInputAudio {
				return true
			}
		}
	}
	return false
}

// convertResourceItem 单项资源转换
func convertResourceItem(item ContentItem, origin string, _ *urlExtractBudget, _ string) ContentItem {
	switch item.Type {
	case ContentText:
		return ContentItem{Type: ContentText, Text: textWithResourceHandles(item.Text, origin, nil, "")}
	case ContentImageURL:
		if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
			return ContentItem{Type: ContentText, Text: ""}
		}
		return convertImageResource(item.ImageURL.URL, origin, nil, "")
	case ContentVideoURL:
		source := ""
		if item.VideoURL != nil {
			source = item.VideoURL.URL
		}
		return ContentItem{Type: ContentText, Text: videoHandleText(source, origin)}
	case ContentAudioURL:
		source := ""
		if item.AudioURL != nil {
			source = item.AudioURL.URL
		}
		return ContentItem{Type: ContentText, Text: audioHandleText(source, mediaName(source, ".wav"), origin)}
	case ContentInputAudio:
		if item.InputAudio == nil {
			return ContentItem{Type: ContentText, Text: ""}
		}
		format := item.InputAudio.Format
		if format == "" {
			format = "wav"
		}
		source := "data:audio/" + format + ";base64," + item.InputAudio.Data
		return ContentItem{Type: ContentText, Text: audioHandleText(source, mediaName(source, ".wav"), clientUploadOrigin)}
	default:
		return ContentItem{Type: ContentText, Text: ""}
	}
}

// convertImageResource 图片类：已带视觉句柄的原样保留；动图 → 句柄；静图 → 视觉缩影
func convertImageResource(source, origin string, _ *urlExtractBudget, _ string) ContentItem {
	source = resource.UpgradeCDNScheme(source)
	if tag := handleFromVisionURL(source); tag != "" {
		return ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: source}}
	}
	if isVideoSource(source) {
		return ContentItem{Type: ContentText, Text: videoHandleText(source, origin)}
	}
	animated, err := isAnimatedImageSource(source)
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源转换] 动图判定失败（%s）: %v", resource.ShortURLForLog(source), err)
	}
	if err == nil && animated {
		return ContentItem{Type: ContentText, Text: animatedHandleText(source, origin)}
	}
	name := "image" + guessImageExt(source)
	admitted := admitImageDataURI(source, name)
	if admitted == nil || admitted.Handle == "" || admitted.BlobPath == "" {
		if admitted != nil && admitted.Handle != "" {
			forgetResourceSource(admitted.Handle)
		}
		forgetResourceSource(source)
		return ContentItem{Type: ContentText, Text: ""}
	}
	return ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: visionReference(admitted.Handle, admitted.BlobPath)}}
}

// textWithResourceHandles 文本里的网络地址 / BV 号 → 句柄，只登记不解析
func textWithResourceHandles(text, origin string, _ *urlExtractBudget, _ string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	converted := textURLPattern.ReplaceAllStringFunc(text, func(match string) string {
		trimmed := strings.TrimRight(match, urlTrailingPunct)
		if trimmed == "" {
			return match
		}
		if !urlDeservesHandle(trimmed) {
			return match
		}
		handle := urlHandleText(trimmed, "", origin, nil, "")
		if handle == trimmed || handle == "" {
			return match
		}
		return handle + match[len(trimmed):]
	})
	return textBVPattern.ReplaceAllStringFunc(converted, func(match string) string {
		return urlHandleText(match, ".mp4", origin, nil, "")
	})
}

// urlDeservesHandle 文本里的 http(s) 地址与 BV 号一律转句柄
func urlDeservesHandle(rawURL string) bool {
	if resource.BilibiliBvid(rawURL) != "" {
		return true
	}
	return isHTTPURL(rawURL)
}

// urlHandleText 网络视频 / 网页 / BV 号 → 句柄；接入阶段不做任何内容解析
func urlHandleText(source, fallbackExt, origin string, _ *urlExtractBudget, _ string) string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return ""
	}
	url := mediaURLForSource(raw, fallbackExt)
	if url == "" {
		return unavailableLinkText
	}
	result, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:    url,
		Origin: origin,
	})
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源转换] 链接入库失败: %v", err)
		return unavailableLinkText
	}
	if result.Handle == "" {
		return unavailableLinkText
	}
	return result.Handle
}

// animatedHandleText 动图 → @G 句柄；已有解析文本时一并带上
func animatedHandleText(source, origin string) string {
	tag := handleFromVisionURL(source)
	admittedURL := ""
	if tag == "" {
		if admitted := admitImageDataURI(source, "image"+guessImageExt(source)); admitted != nil {
			tag = admitted.Handle
			admittedURL = admitted.URL
		}
	}
	if tag == "" {
		return mediaName(source, ".gif")
	}
	if admittedURL != "" {
		if summary := strings.TrimSpace(resource.ParseTextOf(admittedURL)); summary != "" {
			return tag + "（动图：" + truncateRunes(summary, 1200) + "）"
		}
	}
	return tag
}

// videoHandleText 视频 → @V 句柄：B 站指向完整页面 URL，其余先下载落盘再绑定本地 URL
func videoHandleText(source, origin string) string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if resource.BilibiliBvid(raw) != "" || strings.Contains(lower, "bilibili.com/video/") || strings.Contains(lower, "b23.tv/") {
		return urlHandleText(raw, ".mp4", origin, nil, "")
	}
	name := mediaName(raw, ".mp4")
	admitted := admitMedia(raw, name, origin, resource.KindVideo)
	if admitted == nil || admitted.Handle == "" {
		logger.Warn(lunarCoreModule, "[资源转换] 视频落盘失败，退回远程句柄: %s", resource.ShortURLForLog(raw))
		if handle := urlHandleText(raw, ".mp4", origin, nil, ""); handle != "" && handle != unavailableLinkText {
			return handle
		}
		return name
	}
	return admitted.Handle
}

// audioHandleText 音频 → @A 句柄；直出模式下直接返回转写正文
func audioHandleText(source, name, origin string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	admitted := admitMedia(source, name, origin, resource.KindAudio)
	if admitted == nil || admitted.Handle == "" {
		if handle := urlHandleText(source, "", origin, nil, ""); handle != "" && handle != unavailableLinkText {
			return handle
		}
		return name
	}
	if audioDirectTextEnabled {
		if text := audioParseTextOf(admitted.Handle); text != "" {
			return text
		}
		if text, err := transcribeAudioResource(admitted.Handle); err == nil && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return admitted.Handle
}

// documentHandleText 文件 / 代码 / 解析失败 / 未知类型 → @D 文件句柄
func documentHandleText(source, name, origin string) string {
	if strings.TrimSpace(source) != "" {
		if media := admitMedia(source, name, origin, resource.KindDocument); media != nil {
			return media.Handle
		}
	}
	return name
}

// mediaURLForSource 把媒体来源整理成可直接登记的地址（本地地址补全为服务地址）
func mediaURLForSource(source, _ string) string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if bvid := resource.BilibiliBvid(raw); bvid != "" && !strings.Contains(lower, "http") {
		return "https://www.bilibili.com/video/" + bvid
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	if strings.HasPrefix(lower, "data:") {
		kind := "video"
		if strings.HasPrefix(lower, "data:audio/") {
			kind = "voice"
		}
		if rel := saveBase64Media(raw, "", kind); rel != "" {
			return resource.FileReadURL(rel)
		}
		return ""
	}
	if rel := localPathToReadURL(raw); rel != "" {
		return rel
	}
	return ""
}

// isVideoSource 判定来源是否为视频（视频扩展名 / data:video / B 站地址与 BV 号）
func isVideoSource(source string) bool {
	lower := strings.ToLower(strings.TrimSpace(source))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "data:video/") || hasVideoExt(pathPartOf(lower)) {
		return true
	}
	if resource.BilibiliBvid(source) != "" {
		return true
	}
	return strings.Contains(lower, "bilibili.com/video/") || strings.Contains(lower, "b23.tv/")
}
