package DialogueBridging

// 入站媒体 → 多模态信封（QQ 写侧唯一构造点，群聊与私聊共用）
//
// 每类信封只用同名专用字段承载载荷；图片类型先按文件头魔数嗅探真实字节，
// 拿不到字节时才退回 NapCat 消息段声明或来源方给出的文件名扩展名。

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"strings"
)

// mediaSourceOf 群聊与私聊共用同一来源判定
func mediaSourceOf(groupID int64) string {
	if groupID != 0 {
		return mediaobject.SourceQQGroup
	}
	return mediaobject.SourceQQPrivate
}

// mediaRef 构造信封公共引用：远程 / data / file 地址进 url，本地路径进 path
func mediaRef(groupID int64, ref, mimeType, name string) mediaobject.Ref {
	payload := mediaobject.Ref{Source: mediaSourceOf(groupID), MIME: mimeType, Name: name}
	if isURLReference(ref) {
		payload.URL = ref
	} else {
		payload.Path = ref
	}
	return payload
}

// isURLReference 判断引用是否属于 url 形态（其余按本地路径处理）
func isURLReference(ref string) bool {
	lower := strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "file://")
}

// imageEnvelope 按最终确定的类型（嗅探结果优先）构造静态图 / 动图信封
func imageEnvelope(groupID int64, ref, mimeType, name string, animated bool) (string, any) {
	payload := mediaRef(groupID, ref, mimeType, name)
	if animated || mediaobject.IsAnimatedImageMIME(mimeType) {
		return mediaobject.EnvelopeImageAnimated, &mediaobject.ImageAnimatedPayload{Ref: payload}
	}
	return mediaobject.EnvelopeImageStatic, &mediaobject.ImageStaticPayload{Ref: payload}
}

// buildImageSegmentEnvelope image 段：先按字节嗅探真实类型，拿不到字节再按消息段声明
func buildImageSegmentEnvelope(groupID int64, data napcat.ImageData, ref string) (string, any) {
	name := mediaDisplayName("图片", data.File, ref)
	if mimeType, animated, resolved, ok := sniffImageReference(ref); ok {
		return imageEnvelope(groupID, resolved, mimeType, name, animated)
	}
	// 拿不到字节（多为远程直链下载失败）：退回消息段声明并记录
	if isHTTPLink(ref) {
		logger.SubWarn("LunarCore", "Napcat", "图片嗅探失败，按消息段声明兜底: name=%s", name)
	}
	mimeType := mediaobject.MIMETypeOfDataURI(ref)
	if mimeType == "" {
		mimeType = mediaobject.MIMETypeOfExt(data.File)
	}
	return imageEnvelope(groupID, ref, mimeType, name, data.SubType == 1 || data.EmojiID != "")
}

// buildMFaceEnvelope mface 段：先嗅字节，拿不到字节时按段语义声明动图
func buildMFaceEnvelope(groupID int64, data napcat.MFaceData, ref string) (string, any) {
	name := mediaDisplayName("表情", ref)
	if mimeType, animated, resolved, ok := sniffImageReference(ref); ok {
		return imageEnvelope(groupID, resolved, mimeType, name, animated)
	}
	if isHTTPLink(ref) {
		logger.SubWarn("LunarCore", "Napcat", "商城表情嗅探失败，按段类型声明动图: ref=%s", ref)
	}
	mimeType := mediaobject.MIMETypeOfDataURI(ref)
	if mimeType == "" {
		mimeType = mediaobject.MIMETypeOfExt(data.Path)
	}
	return imageEnvelope(groupID, ref, mimeType, name, true)
}

// buildFileImageEnvelope 文件通道图片：MIME 与动画能力由字节嗅探结果给出
func buildFileImageEnvelope(groupID int64, ref, declaredName, mimeType string, animated bool) (string, any) {
	return imageEnvelope(groupID, ref, mimeType, mediaDisplayName("图片", declaredName), animated)
}

// buildVideoEnvelope video 段：MIME 取声明（data URI 头 > 文件名扩展名）
func buildVideoEnvelope(groupID int64, data napcat.VideoData, ref string) (string, any) {
	mimeType := mediaobject.MIMETypeOfDataURI(ref)
	if mimeType == "" {
		mimeType = mediaobject.MIMETypeOfExt(data.File)
	}
	return mediaobject.EnvelopeVideo, &mediaobject.VideoPayload{
		Ref: mediaRef(groupID, ref, mimeType, mediaDisplayName("视频", data.File, ref)),
	}
}

// buildWebpageEnvelope 文本中的链接 → 网页信封（名字留空，由接收侧按链接补）
func buildWebpageEnvelope(groupID int64, rawURL string) (string, any) {
	return newWebpageEnvelope(groupID, rawURL, "")
}

// buildCardWebpageEnvelope 分享卡片 → 网页信封：名字取卡片标题，没有标题用中性名；
// 只有 http/https 链接才下发，其余协议视为拿不到链接。
func buildCardWebpageEnvelope(groupID int64, rawURL, title string) (string, any) {
	rawURL = strings.TrimSpace(rawURL)
	if !isHTTPLink(rawURL) {
		return "", nil
	}
	name := strings.TrimSpace(title)
	if name == "" {
		name = "网页链接"
	}
	return newWebpageEnvelope(groupID, rawURL, name)
}

// newWebpageEnvelope 组装网页信封；主机命中共享视频平台域名表时声明 web_video
func newWebpageEnvelope(groupID int64, rawURL, name string) (string, any) {
	payload := &mediaobject.WebpagePayload{Ref: mediaRef(groupID, rawURL, "", name)}
	if mediaobject.IsVideoPlatformURL(rawURL) {
		payload.Platform = mediaobject.WebPlatformVideo
	}
	return mediaobject.EnvelopeWebpage, payload
}

// isHTTPLink 判断是否为可下发的 http/https 链接
func isHTTPLink(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// buildAudioObject record 段：音频走智能体语音管线，MIME 取声明（转码结果为 data URI 头）
func buildAudioObject(groupID int64, data napcat.RecordData, ref string) mediaobject.Object {
	mimeType := mediaobject.MIMETypeOfDataURI(ref)
	if mimeType == "" {
		mimeType = mediaobject.MIMETypeOfExt(data.File)
	}
	return mediaobject.Object{
		Type:   mediaobject.TypeAudio,
		Source: mediaSourceOf(groupID),
		URL:    ref,
		MIME:   mimeType,
		Name:   mediaDisplayName("语音", data.File, ref),
	}
}

// appendEnvelopeItem 追加内容项：信封类型与专用载荷字段同名
func appendEnvelopeItem(contentArray *[]map[string]any, contentStr *string, envelope string, payload any) {
	markMultimedia(contentArray, contentStr)
	*contentArray = append(*contentArray, map[string]any{
		"type":   envelope,
		envelope: payload,
	})
}

// appendTextWithWebpages 追加文本，并把其中的链接抽成 webpage 信封；返回是否抽出了链接
func appendTextWithWebpages(contentArray *[]map[string]any, contentStr *string, groupID int64, text string) bool {
	plain, urls := splitTextURLs(text)
	if plain != "" {
		appendContent(contentArray, contentStr, plain)
	}
	for _, rawURL := range urls {
		envelope, payload := buildWebpageEnvelope(groupID, rawURL)
		appendEnvelopeItem(contentArray, contentStr, envelope, payload)
	}
	return len(urls) > 0
}

// splitTextURLs 把文本拆成「去掉链接的纯文本」与链接列表
func splitTextURLs(text string) (string, []string) {
	matches := textURLPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	var plain strings.Builder
	urls := make([]string, 0, len(matches))
	last := 0
	for _, match := range matches {
		plain.WriteString(text[last:match[0]])
		urls = append(urls, trimURLTail(text[match[0]:match[1]]))
		last = match[1]
	}
	plain.WriteString(text[last:])
	return plain.String(), urls
}

// trimURLTail 去掉链接尾部常见的句读符号
func trimURLTail(raw string) string {
	return strings.TrimRight(raw, ".,;:!?)]}>'\"")
}
