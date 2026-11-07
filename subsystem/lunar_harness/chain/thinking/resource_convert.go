package thinking

// resource_convert.go 未读上下文的统一资源清洗（ResourcesConversionHandle）
//
// 入站多模态信封已在 prepareInboundMessages 中完成「声明校验 + 落盘 + 字节校验」，
// 这里只处理清洗后的内部内容项：
//   - 静态图 → 视觉缩影链接（句柄藏在 tag 参数里）；
//   - 音频 → 句柄 + 转写文本；
//   - 引用消息 → 引用快照文本；
//   - 文本保持纯文本：URL 抽取由发送侧负责，接收侧不再做任何猜测。
//
// 旧信封（image_url / video_url / audio_url / input_audio）在入站阶段即判定无效，
// 这里不再保留任何兼容分支；历史缓存重放由 cachedToContext 直接生成清洗后的内容。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// ResourcesConversionHandle 未读上下文的统一资源预处理：返回「可直接进模型上下文」的消息副本。
func ResourcesConversionHandle(messages []PostMessage) []PostMessage {
	if len(messages) == 0 {
		return messages
	}
	prepared := prepareInboundMessages(messages)
	converted := make([]PostMessage, len(prepared.Messages))
	for index := range prepared.Messages {
		converted[index] = convertMessageResources(prepared.Messages[index], false)
	}
	// 本批音频已在接入阶段全部转写：统一卸载 ASR，避免逐条加载/卸载抖动
	if batchHasAudio(messages) {
		resource.UnloadASRModel()
	}
	return converted
}

// convertMessageResources 单条消息的内部内容项清洗
//
// replay=true 表示历史消息重放：视频只登记句柄，不重新下载落盘。
func convertMessageResources(message PostMessage, replay bool) PostMessage {
	if _, ok := message.Content.(string); ok {
		// 工具回传的文本与用户纯文本都保持原样
		return message
	}
	items := asItems(message.Content)
	if len(items) == 0 {
		return message
	}
	out := make([]ContentItem, 0, len(items))
	for _, item := range items {
		out = append(out, convertResourceItem(item, channelOrigin, replay))
	}
	message.Content = out
	return message
}

// batchHasAudio 本批未读上下文是否包含音频资源（引用块仅在被引用消息含音频句柄时计为音频）
func batchHasAudio(messages []PostMessage) bool {
	for _, message := range messages {
		for _, item := range asItems(message.Content) {
			switch item.Type {
			case ContentAudio, ContentAudioHandle:
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

// convertResourceItem 单项内部资源转换
func convertResourceItem(item ContentItem, origin string, replay bool) ContentItem {
	switch item.Type {
	case ContentText:
		return item
	case ContentImageURL:
		if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
			return ContentItem{Type: ContentText, Text: ""}
		}
		// base64 模式下的视觉缩影没有 tag 参数：已是清洗产物且带句柄时原样保留，
		// 避免二次转换把缩影再登记成另一个资源、导致近期与远期句柄不一致
		if item.ResourceHandle != "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.ImageURL.URL)), "data:") {
			return item
		}
		return convertImageResource(item.ImageURL, origin)
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

// convertImageResource 内部 image_url 只承载静态图：已带视觉句柄的原样保留，其余落盘为缩影
func convertImageResource(payload *URLPayload, origin string) ContentItem {
	source := resource.UpgradeCDNScheme(payload.URL)
	if tag := handleFromVisionURL(source); tag != "" {
		return ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: source}, ResourceHandle: tag}
	}
	name := imageNameFor(source, resource.KindImage, payload.MIME)
	admitted, admitErr := admitImageSource(source, name, resource.KindImage)
	if admitted == nil || admitted.Handle == "" || admitted.BlobPath == "" {
		if admitted != nil && admitted.Handle != "" {
			forgetResourceSource(admitted.Handle)
		}
		forgetResourceSource(source)
		return ContentItem{Type: ContentText, Text: imageFailureText(admitErr)}
	}
	if err := resource.VerifyStored(resource.KindImage, resource.AbsPathOfBlob(admitted.BlobPath), payload.MIME); err != nil {
		if !admitted.Deduped {
			_ = resource.Delete(admitted.Handle)
		}
		return ContentItem{Type: ContentText, Text: imageFailureText(err)}
	}
	return ContentItem{
		Type:           ContentImageURL,
		ImageURL:       &URLPayload{URL: visionReference(admitted.Handle, admitted.BlobPath)},
		ResourceHandle: admitted.Handle,
	}
}

// imageFailureText 图片读取失败的模型可见说明：固定标注媒体种类为图片，并带上真实原因
func imageFailureText(err error) string {
	reason := "资源未能下载或不是可识别的图片"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		reason = strings.TrimSpace(err.Error())
	}
	return "（图片读取失败：" + textutil.TruncateRunes(reason, 200) + "）"
}

// urlHandleText 链接按显式声明登记句柄；kindHint 由调用方给出
func urlHandleText(source, kindHint, origin string) string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return ""
	}
	result, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:      raw,
		Origin:   origin,
		KindHint: kindHint,
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

// videoHandleText 视频 → @V 句柄。
//
// 平台页面（发送侧声明 web_video，共享域名表核对）只登记远程句柄；
// 其余来源按声明类型下载落盘。这里不再比对域名或扩展名猜测类型。
func videoHandleText(source, origin string, replay bool) string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return ""
	}
	if mediaobject.IsVideoPlatformURL(raw) {
		return urlHandleText(raw, resource.KindVideo, origin)
	}
	if replay {
		return urlHandleText(raw, resource.KindVideo, origin)
	}
	name := mediaName(raw, ".mp4")
	admitted := admitMedia(raw, name, origin, resource.KindVideo)
	if admitted == nil || admitted.Handle == "" {
		logger.Warn(lunarCoreModule, "[资源转换] 视频落盘失败: %s", resource.ShortURLForLog(raw))
		return urlHandleText(raw, resource.KindVideo, origin)
	}
	if err := resource.VerifyStored(resource.KindVideo, resource.AbsPathOfBlob(admitted.BlobPath), ""); err != nil {
		if !admitted.Deduped {
			_ = resource.Delete(admitted.Handle)
		}
		logger.Warn(lunarCoreModule, "[资源转换] 视频声明校验失败: %v", err)
		return urlHandleText(raw, resource.KindVideo, origin)
	}
	return admitted.Handle
}

// audioHandleText 音频 → 落盘 audios + 转写文本 + @A 句柄
func audioHandleText(source, name, origin string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	handle, text, err := resource.IngestAudio(source, name, origin)
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
