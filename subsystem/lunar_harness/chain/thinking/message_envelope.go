package thinking

// message_envelope.go 入站多模态信封的接收链路
//
// 发送侧定义类型，后端只做强制校验：
//  1. 结构 / 来源 / 声明 MIME / 网页平台域名必须自洽（mediaobject.ValidateEnvelope）；
//  2. 落盘后字节魔数必须与声明一致（resource.VerifyStored）；
//  3. 校验或落盘失败 → 只丢弃该信封并记录 reason 与计数，同条消息其余内容照常处理。
//
// 本文件不做任何类型推断：URL 后缀、主机名、扩展名都不参与类型判定。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// inboundResult 一批入站消息的准备结果
type inboundResult struct {
	// Messages 清洗后的消息
	Messages []PostMessage
	// Handles 音频句柄（按消息内出现顺序；失败位不占槽）
	Handles []string
	// Total 本批媒体信封总数
	Total int
	// Invalid 被丢弃的信封数
	Invalid int
}

// prepareInboundMessages 逐信封校验、落盘、清洗；无效信封单独丢弃。
func prepareInboundMessages(messages []PostMessage) inboundResult {
	result := inboundResult{Messages: make([]PostMessage, 0, len(messages))}
	for index := range messages {
		result.Messages = append(result.Messages, prepareInboundMessage(messages[index], &result))
	}
	if result.Total > 0 {
		logger.Info(lunarCoreModule, "[信封] 本批 %d 个，丢弃 %d 个", result.Total, result.Invalid)
	}
	return result
}

// prepareInboundMessage 单条消息的逐信封处理
func prepareInboundMessage(message PostMessage, result *inboundResult) PostMessage {
	if _, ok := message.Content.(string); ok {
		return message
	}
	items := asItems(message.Content)
	if len(items) == 0 {
		return message
	}
	out := make([]ContentItem, 0, len(items))
	invalid := 0
	for _, item := range items {
		if reason, legacy := legacyInboundEnvelope(item); legacy {
			invalid++
			result.Total++
			result.Invalid++
			logger.Warn(lunarCoreModule, "[信封] 丢弃 type=%s source=%s name=%s reason=%s",
				item.Type, envelopeSource(item), envelopeName(item), reason)
			continue
		}
		if !isInboundEnvelope(item.Type) {
			out = append(out, item)
			continue
		}
		result.Total++
		cleaned, handle, err := ingestEnvelope(item)
		if err != nil {
			invalid++
			result.Invalid++
			logger.Warn(lunarCoreModule, "[信封] 丢弃 type=%s source=%s name=%s reason=%v",
				item.Type, envelopeSource(item), envelopeName(item), err)
			continue
		}
		if handle != "" {
			result.Handles = append(result.Handles, handle)
		}
		out = append(out, cleaned)
	}
	// 整条消息只剩被丢弃的信封：保留中性占位，避免消息直接消失
	if len(out) == 0 && invalid > 0 {
		out = append(out, ContentItem{Type: ContentText, Text: fmt.Sprintf("（有 %d 个媒体信封未能接收）", invalid)})
	}
	message.Content = out
	return message
}

// ingestEnvelope 单个信封：声明校验 → 落盘 → 字节校验 → 清洗
func ingestEnvelope(item ContentItem) (ContentItem, string, error) {
	envelope := toEnvelope(item)
	kind, ref, err := mediaobject.ValidateEnvelope(envelope)
	if err != nil {
		return ContentItem{}, "", fmt.Errorf("声明校验失败：%w", err)
	}
	switch kind {
	case mediaobject.EnvelopeImageStatic:
		return admitImageEnvelope(ref, resource.KindImage)
	case mediaobject.EnvelopeImageAnimated:
		return admitImageEnvelope(ref, resource.KindAnimated)
	case mediaobject.EnvelopeVideo:
		return admitStoredEnvelope(ref, resource.KindVideo, ".mp4")
	case mediaobject.EnvelopeFile:
		return admitStoredEnvelope(ref, resource.KindDocument, "")
	case mediaobject.EnvelopeAudio:
		return admitAudioEnvelope(ref)
	case mediaobject.EnvelopeWebpage:
		return admitWebpageEnvelope(envelope.Webpage)
	default:
		return ContentItem{}, "", fmt.Errorf("未知信封类型 %q", kind)
	}
}

// admitImageEnvelope 静态图 → 视觉缩影；动图 → G 句柄文本
func admitImageEnvelope(ref *mediaobject.Ref, kindCode string) (ContentItem, string, error) {
	admitted, err := admitEnvelopeSource(ref, kindCode)
	if err != nil {
		return ContentItem{}, "", err
	}
	if err := resource.VerifyStored(kindCode, resource.AbsPathOfBlob(admitted.BlobPath), ref.MIME); err != nil {
		dropAdmitted(admitted)
		return ContentItem{}, "", err
	}
	if kindCode == resource.KindAnimated {
		return textItem(admitted.Handle), "", nil
	}
	return ContentItem{
		Type:           ContentImageURL,
		ImageURL:       &URLPayload{Type: mediaobject.TypeImageStatic, URL: visionReference(admitted.Handle, admitted.BlobPath)},
		ResourceHandle: admitted.Handle,
	}, "", nil
}

// admitStoredEnvelope 视频 / 文件：落盘 + 字节校验 → 句柄文本
func admitStoredEnvelope(ref *mediaobject.Ref, kindCode, fallbackExt string) (ContentItem, string, error) {
	admitted, err := admitEnvelopeSource(ref, kindCode)
	if err != nil {
		return ContentItem{}, "", err
	}
	if err := resource.VerifyStored(kindCode, resource.AbsPathOfBlob(admitted.BlobPath), ref.MIME); err != nil {
		dropAdmitted(admitted)
		return ContentItem{}, "", err
	}
	return textItem(admitted.Handle), "", nil
}

// admitAudioEnvelope 音频：落盘 audios → 转写 → A 句柄
func admitAudioEnvelope(ref *mediaobject.Ref) (ContentItem, string, error) {
	source := ref.Reference()
	name := envelopeFileName(ref, ".mp3")
	admitted, text, err := resource.AdmitAudio(source, name, envelopeOrigin(ref.Source), ref.MIME, classifySourceKind(source))
	if err != nil {
		dropAdmitted(admitted)
		return ContentItem{}, "", err
	}
	if admitted.Handle == "" {
		return ContentItem{}, "", fmt.Errorf("未生成可用音频句柄")
	}
	return audioHandleContextItem(admitted.Handle, text, name), admitted.Handle, nil
}

// admitWebpageEnvelope 网页：先做可达性校验，再按平台声明登记 U / V 句柄
func admitWebpageEnvelope(webpage *mediaobject.WebpagePayload) (ContentItem, string, error) {
	if webpage == nil {
		return ContentItem{}, "", fmt.Errorf("网页载荷为空")
	}
	source := strings.TrimSpace(webpage.URL)
	// 已登记过的链接不再逐轮探测可达性（历史消息重放会反复带来同一链接）
	if existing, ok := resource.FindByURL(source); ok && existing.Handle != "" {
		return textItem(existing.Handle), "", nil
	}
	if err := resource.CheckReachable(source); err != nil {
		return ContentItem{}, "", fmt.Errorf("网页不可达：%w", err)
	}
	kindCode := resource.KindURL
	if strings.TrimSpace(webpage.Platform) == mediaobject.WebPlatformVideo {
		kindCode = resource.KindVideo
	}
	result, err := resource.AdmitURL(resource.URLAdmitInput{
		URL:      source,
		Title:    strings.TrimSpace(webpage.Name),
		Name:     strings.TrimSpace(webpage.Name),
		Origin:   envelopeOrigin(webpage.Source),
		KindHint: kindCode,
	})
	if err != nil {
		return ContentItem{}, "", err
	}
	if result.Handle == "" {
		return ContentItem{}, "", fmt.Errorf("未生成可用网页句柄")
	}
	return textItem(result.Handle), "", nil
}

// admitEnvelopeSource 统一落盘入口：类型码完全来自信封声明
func admitEnvelopeSource(ref *mediaobject.Ref, kindCode string) (resource.AdmitResult, error) {
	source := ref.Reference()
	if source == "" {
		return resource.AdmitResult{}, fmt.Errorf("来源引用为空")
	}
	admitted, err := resource.Admit(resource.AdmitInput{
		SourceKind: classifySourceKind(source),
		Source:     source,
		Name:       envelopeFileName(ref, extForEnvelope(kindCode)),
		KindHint:   kindCode,
		MIME:       ref.MIME,
		Origin:     envelopeOrigin(ref.Source),
		Dedup:      true,
	})
	if err != nil {
		return resource.AdmitResult{}, err
	}
	if admitted.Handle == "" || admitted.BlobPath == "" {
		dropAdmitted(admitted)
		return resource.AdmitResult{}, fmt.Errorf("未生成可用句柄")
	}
	return admitted, nil
}

// dropAdmitted 校验失败时只回收本次新建的资源，复用既有资源时不动
func dropAdmitted(admitted resource.AdmitResult) {
	if admitted.Handle == "" || admitted.Deduped {
		return
	}
	_ = resource.Delete(admitted.Handle)
}

// envelopeFileName 信封声明的名字优先，其次按引用推断
func envelopeFileName(ref *mediaobject.Ref, fallbackExt string) string {
	if name := strings.TrimSpace(ref.Name); name != "" {
		return name
	}
	return mediaName(ref.Reference(), fallbackExt)
}

// envelopeOrigin 来源语义 → 资源目录 origin
func envelopeOrigin(source string) string {
	if source == mediaobject.SourceClientUpload {
		return resource.OriginClientUpload
	}
	return resource.OriginChannel
}

// extForEnvelope 仅用于缺省文件名的命名，不参与类型判定
func extForEnvelope(kindCode string) string {
	switch kindCode {
	case resource.KindImage:
		return ".jpg"
	case resource.KindAnimated:
		return ".gif"
	case resource.KindVideo:
		return ".mp4"
	case resource.KindAudio:
		return ".mp3"
	default:
		return ""
	}
}

// textItem 句柄文本项
func textItem(handle string) ContentItem {
	return ContentItem{Type: ContentText, Text: handle}
}

// toEnvelope 把内容项还原为信封，交给统一校验
func toEnvelope(item ContentItem) *mediaobject.Envelope {
	return &mediaobject.Envelope{
		Type:          item.Type,
		Text:          item.Text,
		ImageStatic:   item.ImageStatic,
		ImageAnimated: item.ImageAnimated,
		Video:         item.Video,
		Audio:         item.Audio,
		File:          item.File,
		Webpage:       item.Webpage,
	}
}

func isInboundEnvelope(contentType string) bool {
	switch contentType {
	case ContentImageStatic, ContentImageAnimated, ContentVideo, ContentAudio, ContentFile, ContentWebpage:
		return true
	default:
		return false
	}
}

// legacyInboundEnvelope 旧信封拦截：发送侧必须改用专用信封字段。
//
// 唯一放行的是服务端自己生成的视觉缩影：ResourceHandle 的 json tag 为 "-"，
// 根本不参与 JSON 编解码，发送侧无法伪造。data URI、/file/read/ 路径与裸网络
// image_url 一律无效，避免「旧信封仍能走通」的第二套协议。
func legacyInboundEnvelope(item ContentItem) (string, bool) {
	switch item.Type {
	case ContentVideoURL, ContentAudioURL, ContentInputAudio:
		return "旧信封已禁用，请改用 video / audio / file 专用信封", true
	case ContentImageURL:
		if item.ResourceHandle != "" {
			return "", false
		}
		return "旧信封 image_url 已禁用，请改用 image_static / image_animated", true
	default:
		return "", false
	}
}

func envelopeRef(item ContentItem) *mediaobject.Ref {
	if _, ref, err := toEnvelope(item).Declared(); err == nil {
		return ref
	}
	return nil
}

func envelopeSource(item ContentItem) string {
	if ref := envelopeRef(item); ref != nil {
		return ref.Source
	}
	return ""
}

func envelopeName(item ContentItem) string {
	if ref := envelopeRef(item); ref != nil {
		return ref.Name
	}
	return ""
}
