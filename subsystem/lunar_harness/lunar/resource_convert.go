package lunar

// resource_convert.go 统一的资源→句柄预处理（ResourcesConversionHandle）
//
// 对话者取未读上下文时，除普通文本外的资源一律先经这里换成句柄：
//   图片        → 视觉缩影地址（@I 句柄藏在地址的 tag 参数里）
//   动图/视频   → 句柄文本（动图 @I，视频/网页/BV 号 @U）
//   音频        → 转写后回填「句柄 + 转写文本」（ASR 在整批转写后统一卸载）
//   文件/代码/未知类型/解析失败 → @D 文档句柄
// 任何分支都不产出「[图片]」「[文件]」这类类型占位符：拿不到句柄时退化为文件名。

import (
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
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
//
// 调用方只需在把消息写进上下文之前调用一次；本函数不读写未读队列，也不改动入参。
// 链接（网页 / B 站视频 / BV 号）在没有缓存摘要时由该函数现场提炼，
// 模型切换发生在 WebSearch / BilibiliViewer 内部，不占用月华的主模型。
func ResourcesConversionHandle(messages []PostMessage) []PostMessage {
	if len(messages) == 0 {
		return messages
	}

	converted := make([]PostMessage, len(messages))
	slices := make([][]ContentItem, len(messages))
	tasks := make([]ResourceAudioTask, 0)
	budget := &urlExtractBudget{}
	focus := batchURLFocus(messages)
	for index := range messages {
		converted[index] = messages[index]
		if text, ok := messages[index].Content.(string); ok {
			// 工具回传的文本（检索报告等）保持原样：里面的链接是结论的一部分，不再改写
			if messages[index].Role == RoleTool {
				continue
			}
			converted[index].Content = textWithResourceHandles(text, channelOrigin, budget, focus)
			continue
		}
		items := asItems(messages[index].Content)
		if len(items) == 0 {
			continue
		}
		out := make([]ContentItem, 0, len(items))
		for _, item := range items {
			result, task := convertResourceItem(item, channelOrigin, budget, focus)
			if task != nil {
				task.Message = index
				task.Slot = len(out)
				tasks = append(tasks, *task)
			}
			out = append(out, result)
		}
		slices[index] = out
		converted[index].Content = out
	}

	if len(tasks) > 0 {
		texts := make([]string, len(tasks))
		for i, task := range tasks {
			texts[i] = resourceAudioText(task.Source, task.Name, task.Origin)
		}
		unloadASRModel()
		for i, task := range tasks {
			slices[task.Message][task.Slot].Text = texts[i]
		}
	}
	return converted
}

// convertResourceItem 单项资源转换；音频返回待转写任务（文本槽位稍后回填）
func convertResourceItem(item ContentItem, origin string, budget *urlExtractBudget, focus string) (ContentItem, *ResourceAudioTask) {
	switch item.Type {
	case ContentText:
		return ContentItem{Type: ContentText, Text: textWithResourceHandles(item.Text, origin, budget, focus)}, nil
	case ContentImageURL:
		if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
			return ContentItem{Type: ContentText, Text: "（空的图片引用，已忽略）"}, nil
		}
		return convertImageResource(item.ImageURL.URL, origin, budget, focus), nil
	case ContentVideoURL:
		source := ""
		if item.VideoURL != nil {
			source = item.VideoURL.URL
		}
		return ContentItem{Type: ContentText, Text: urlHandleText(source, ".mp4", origin, budget, focus)}, nil
	case ContentAudioURL:
		source := ""
		if item.AudioURL != nil {
			source = item.AudioURL.URL
		}
		return ContentItem{Type: ContentText, Text: ""}, &ResourceAudioTask{
			Source: source, Name: mediaName(source, ".wav"), Origin: origin,
		}
	case ContentInputAudio:
		if item.InputAudio == nil {
			return ContentItem{Type: ContentText, Text: ""}, nil
		}
		format := item.InputAudio.Format
		if format == "" {
			format = "wav"
		}
		source := "data:audio/" + format + ";base64," + item.InputAudio.Data
		return ContentItem{Type: ContentText, Text: ""}, &ResourceAudioTask{
			Source: source, Name: mediaName(source, ".wav"), Origin: clientUploadOrigin,
		}
	default:
		// 协议内不存在其它类型；真出现未知项时丢弃空文本，不产出类型占位符
		return ContentItem{Type: ContentText, Text: ""}, nil
	}
}

// convertImageResource 图片类：已带视觉句柄的原样保留；动图/视频 → 句柄；静图 → 视觉缩影
func convertImageResource(source, origin string, budget *urlExtractBudget, focus string) ContentItem {
	if tag := handleFromVisionURL(source); tag != "" {
		return ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: source}}
	}
	if isVideoSource(source) {
		return ContentItem{Type: ContentText, Text: urlHandleText(source, ".mp4", origin, budget, focus)}
	}
	animated, err := isAnimatedImageSource(source)
	if err != nil {
		logger.Warn(lunarCoreModule, "[资源转换] 动图判定失败: %v", err)
	}
	if err == nil && animated {
		return ContentItem{Type: ContentText, Text: animatedHandleText(source, origin)}
	}
	name := "image" + guessImageExt(source)
	admitted := admitImageDataURI(source, name)
	if admitted == nil || admitted.Handle == "" || admitted.BlobPath == "" {
		// 图片入库失败：销毁残留句柄并直接跳过，不再退化成文档句柄。
		if admitted != nil && admitted.Handle != "" {
			forgetResourceSource(admitted.Handle)
		}
		forgetResourceSource(source)
		return ContentItem{Type: ContentText, Text: ""}
	}
	return ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: visionReference(admitted.Handle, admitted.BlobPath)}}
}

// textWithResourceHandles 文本里的网络地址 / BV 号 → @U 句柄（未缓存的普通网页保持原样）
func textWithResourceHandles(text, origin string, budget *urlExtractBudget, focus string) string {
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
		handle := urlHandleText(trimmed, "", origin, budget, focus)
		if handle == trimmed || handle == "" {
			return match
		}
		return handle + match[len(trimmed):]
	})
	return textBVPattern.ReplaceAllStringFunc(converted, func(match string) string {
		return urlHandleText(match, ".mp4", origin, budget, focus)
	})
}

// urlDeservesHandle 文本里的 http(s) 地址与 BV 号一律转句柄；
// 没有缓存摘要的会在 urlHandleText 里现场提炼（有配额限制），其余非 http 伪地址原样保留
func urlDeservesHandle(rawURL string) bool {
	if resource.BilibiliBvid(rawURL) != "" {
		return true
	}
	return isHTTPURL(rawURL)
}

// urlHandleText 网络视频 / 网页 / BV 号 → @U 句柄；登记失败给中性提示。
// 首次出现的网页链接没有缓存摘要时先现场提炼；网络视频 / BV 号只登记句柄，不现场理解内容。
func urlHandleText(source, fallbackExt, origin string, budget *urlExtractBudget, focus string) string {
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
	// 没有摘要就现场提炼一次（同一进程内每个地址只尝试一次，失败过的不会反复开页面）
	if budget != nil && budget.used < urlExtractLimit && !hasURLSummary(url) && markURLAttempted(url) {
		title, summary := refreshURLCache(url, budget, focus)
		if summary != "" {
			if updated, updateErr := resource.AdmitURL(resource.URLAdmitInput{
				URL: url, Title: title, Summary: summary, Origin: origin,
			}); updateErr == nil && updated.Handle != "" {
				result = updated
			}
		}
	}
	return result.Handle
}

// animatedHandleText 动图 → @I 句柄；有理解缓存时把感受一并带上
func animatedHandleText(source, origin string) string {
	tag := handleFromVisionURL(source)
	if tag == "" {
		if admitted := admitImageDataURI(source, "image"+guessImageExt(source)); admitted != nil {
			tag = admitted.Handle
		}
	}
	if tag == "" {
		return mediaName(source, ".gif")
	}
	if summary := strings.TrimSpace(getPromptFromKnowledgeAny(animatedSummaryKeys(source))); summary != "" {
		return tag + "（动图：" + truncateRunes(summary, 1200) + "）"
	}
	return tag
}

// documentHandleText 文件 / 代码 / 解析失败 / 未知类型 → @D 文档句柄
func documentHandleText(source, name, origin string) string {
	if strings.TrimSpace(source) != "" {
		if media := admitMedia(source, name, origin, "D"); media != nil {
			return media.Handle
		}
	}
	return name
}

// mediaURLForSource 把媒体来源整理成可直接登记的地址（本地地址补全为服务地址）
func mediaURLForSource(source, fallbackExt string) string {
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
			return config.LocalBaseURL() + rel
		}
		return ""
	}
	if rel := localPathToReadURL(raw); rel != "" {
		return config.LocalBaseURL() + rel
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
