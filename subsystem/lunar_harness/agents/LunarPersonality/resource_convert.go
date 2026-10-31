package LunarPersonality

// resource_convert.go 未读上下文的统一资源预处理（ResourcesConversionHandle）
//
// 规范要求：
//   - 图片 → 视觉缩影链接（句柄藏在 tag 参数里）；
//   - 视频 / 动图 → 句柄文本；音频 → 转写文本 + 悬空句柄（音频本体不落盘）；
//   - 引用消息 → 「引用快照」文本，被引用音频按悬空句柄读取转写；
//   - 文本中的 URL → 句柄文本；接入阶段只登记，解析留到 use_resource；
//   - 文件 / 代码 / 未知类型 → @D 文件句柄。
//
// 任何分支都不产出「[图片]」「[视频]」这类类型占位符：拿不到句柄时退化为文件名。

import (
	"fmt"
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
	// 本批音频已在资源转换时全部转写：统一卸载 ASR，避免逐条加载/卸载抖动
	if batchHasAudio(messages) {
		unloadASRModel()
	}
	return converted
}

// batchHasAudio 本批未读上下文是否包含音频资源（引用块仅在被引用消息含音频句柄时计为音频）
func batchHasAudio(messages []PostMessage) bool {
	for _, message := range messages {
		for _, item := range asItems(message.Content) {
			switch item.Type {
			case ContentAudioURL, ContentInputAudio, ContentAudioHandle:
				return true
			case ContentQuote:
				if item.Quote == nil {
					continue
				}
				for _, quoted := range item.Quote.Messages {
					if strings.TrimSpace(quoted.AudioHandle) != "" {
						return true
					}
				}
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
	case ContentAudioHandle:
		if item.AudioHandle == nil {
			return ContentItem{Type: ContentText, Text: ""}
		}
		return ContentItem{Type: ContentText, Text: audioHandleContextText(item.AudioHandle.Handle, item.AudioHandle.Text)}
	case ContentQuote:
		return ContentItem{Type: ContentText, Text: quoteContextText(item.Quote)}
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
func animatedHandleText(source, _ string) string {
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

// audioHandleText 音频 → 转写文本 + @A 悬空句柄。
// 音频不再落盘：接入阶段直接走 audio_transcriber，句柄只保留带描述的记录。
func audioHandleText(source, name, origin string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	handle, text, err := ingestAudioSource(source, name, origin)
	if err != nil || handle == "" {
		if text != "" {
			return text
		}
		return name
	}
	return audioHandleContextText(handle, text)
}

// quoteContextText 引用消息 → 模型可见的引用文本。
// 被引用消息若包含音频，则按音频句柄读取转写描述（音频句柄的唯一取用时机）。
func quoteContextText(quote *QuotePayload) string {
	if quote == nil || len(quote.Messages) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("[引用消息]")
	for index, message := range quote.Messages {
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("[%d] ", index+1))
		sender := strings.TrimSpace(message.Sender)
		if sender == "" {
			if message.Role == RoleAssistant {
				sender = "月华"
			} else {
				sender = "用户"
			}
		}
		builder.WriteString(sender)
		builder.WriteString("：")
		content := strings.TrimSpace(message.Content)
		if handle := strings.TrimSpace(message.AudioHandle); handle != "" {
			description := audioHandleContextText(handle, message.Text)
			content = strings.TrimSpace(content + " " + description)
		} else if content == "" && strings.TrimSpace(message.Text) != "" {
			content = strings.TrimSpace(message.Text)
		}
		if content == "" {
			content = "（无可读文本）"
		}
		builder.WriteString(content)
	}
	return builder.String()
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
		// 音频一律走转写悬空句柄，绝不落盘；这里仅处理图片/视频数据 URI
		if strings.HasPrefix(lower, "data:audio/") {
			return ""
		}
		if rel := saveBase64Media(raw, "", "video"); rel != "" {
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
