package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/engine/resource"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

func resolveImageURL(imageData napcat.ImageData) string {
	if dataURI := localImageDataURI(imageData.Path); dataURI != "" {
		return dataURI
	}
	if raw := strings.TrimSpace(imageData.URL); raw != "" {
		lower := strings.ToLower(raw)
		if strings.HasPrefix(lower, "data:") {
			return raw
		}
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			// 腾讯图床只接受 https：NapCat 有时下发 http 直链，裸 GET 会被边缘节点拒绝（400）。
			return resource.UpgradeCDNScheme(raw)
		}
		if dataURI := localImageDataURI(raw); dataURI != "" {
			return dataURI
		}
	}
	// 合并转发里的图片常把真实地址写在 file 字段（data URI / 直链 / 本地路径）。
	// 这类引用必须优先按来源直接使用，否则会先拿整串地址去调 get_image，白白超时后丢图。
	if raw := strings.TrimSpace(imageData.File); raw != "" && directFileSource(raw) {
		lower := strings.ToLower(raw)
		if strings.HasPrefix(lower, "data:") {
			return raw
		}
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return resource.UpgradeCDNScheme(raw)
		}
		if dataURI := localImageDataURI(raw); dataURI != "" {
			return dataURI
		}
	}
	if bytes, err := napcat.GetImageContent(imageData.FileID, imageData.File); err == nil && len(bytes) > 0 {
		return imageDataURI(bytes)
	}
	return localImageDataURI(imageData.File)
}

// resolveMFaceImage 取商城表情的图片地址（部分版本附带 url / path，多数只有 summary）
func resolveMFaceImage(mface napcat.MFaceData) string {
	if dataURI := localImageDataURI(mface.Path); dataURI != "" {
		return dataURI
	}
	raw := strings.TrimSpace(mface.URL)
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:") {
		return raw
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	return ""
}

// mfaceDisplayText 商城表情的展示文案
func mfaceDisplayText(mface napcat.MFaceData) string {
	if summary := strings.TrimSpace(mface.Summary); summary != "" {
		return summary
	}
	return "商城表情"
}

// imageDataURI 把图片字节包装为 data URI（MIME 按文件头嗅探，未知回退 PNG）
func imageDataURI(data []byte) string {
	mimeType := sniffImageMIME(data)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// sniffImageReference 嗅探引用指向的真实图片类型。
//
// data URI 直接解码；本地路径读盘；远程直链按需下载一次，成功时把引用改写为
// data URI，保证信封声明的 MIME 与后端校验用的字节完全一致。
// 返回 (MIME, 是否动图, 可用引用, 是否嗅探成功)。
func sniffImageReference(ref string) (string, bool, string, bool) {
	ref = strings.TrimSpace(ref)
	if data := decodeDataURIBytes(ref); len(data) > 0 {
		mimeType, animated := sniffImageBytes(data)
		if mimeType == "" {
			return "", false, ref, false
		}
		// 引用头里的 MIME 可能与真实字节不符（QQ 常把 GIF 标成 image/jpeg），
		// 嗅探成功后按真实格式重建 data URI。
		if !strings.EqualFold(mimeType, mediaobject.MIMETypeOfDataURI(ref)) {
			ref = imageDataURI(data)
		}
		return mimeType, animated, ref, true
	}
	if isHTTPLink(ref) {
		data, err := napcat.FetchFileBytes(ref)
		if err != nil || len(data) == 0 {
			return "", false, ref, false
		}
		mimeType, animated := sniffImageBytes(data)
		if mimeType == "" {
			return "", false, ref, false
		}
		return mimeType, animated, imageDataURI(data), true
	}
	data, err := os.ReadFile(napcat.FileURLPath(ref))
	if err != nil || len(data) == 0 {
		return "", false, ref, false
	}
	mimeType, animated := sniffImageBytes(data)
	if mimeType == "" {
		return "", false, ref, false
	}
	return mimeType, animated, imageDataURI(data), true
}

// sniffImageCandidates 在可读候选里嗅探真实图片类型（只解码 data URI，不额外下载直链）
func sniffImageCandidates(candidates []string) (string, bool, bool) {
	for _, source := range candidates {
		if data := decodeDataURIBytes(source); len(data) > 0 {
			if mimeType, animated := sniffImageBytes(data); mimeType != "" {
				return mimeType, animated, true
			}
		}
	}
	return "", false, false
}

// localImageDataURI 读取本地路径并包装为图片 data URI（非图片或读不到返回空串）
func localImageDataURI(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(napcat.FileURLPath(path))
	if err != nil || len(data) == 0 {
		return ""
	}
	if sniffImageMIME(data) == "" {
		return ""
	}
	return imageDataURI(data)
}

// admitInboundImage 把入站引用中的图片（合并转发 / 回复引用）收进统一资源归档层并返回持久句柄。
//
// 普通消息里的图片会作为多模态内容项直接进入思考链；而引用/转发里的图片不在正文
// 内容项中，必须换成可回引的句柄，模型之后才能用 fetch_resources 查看。此处必须同时
// 兼容两种来源形态：
//   - 远程直链（image.url）：按 url 来源下载；
//   - 仅本地文件引用（image.file）：get_image 下载后得到 data URI，必须按 data 来源解码，
//     不能把 data: 当成 http URL 去请求——那会直接失败并退化成 [图片]。
//
// 直链下载失败时再尝试用 file 引用兜底，避免 QQ CDN 地址过期导致整条转发图全部丢失。
func admitInboundImage(imageData napcat.ImageData) (string, bool) {
	candidates := make([]string, 0, 2)
	if imgURL := resolveImageURL(imageData); imgURL != "" {
		candidates = append(candidates, imgURL)
	}
	// 直链存在时 resolveImageURL 不会读取 file；补一份本地下载结果作为兜底
	if imageData.URL != "" && imageData.File != "" && !strings.HasPrefix(imageData.File, "http://") && !strings.HasPrefix(imageData.File, "https://") {
		if raw, err := napcat.GetImageContent(imageData.FileID, imageData.File); err == nil && len(raw) > 0 {
			candidates = append(candidates, imageDataURI(raw))
		}
	}

	// 有可解码的候选时先按字节判定真实类型：动图入库为动图资源，
	// 静态图仍按图片资源；name 保持来源方原始文件名，不做改名。
	mimeType, animated, sniffed := sniffImageCandidates(candidates)
	for _, source := range candidates {
		sourceKind := admitSourceKind(source)
		kindHint := resource.KindImage
		if sniffed && animated {
			kindHint = resource.KindAnimated
		}
		// image 段本身就是来源方声明的图片类型；扩展名不做改写。
		name := mediaDisplayName("forward", imageData.File)
		// 转发图只为还原对话记录服务：ephemeral 保留等级，配额吃紧时优先回收本体，
		// 句柄降级为描述后依然可引用；同图重复转发按内容去重，不重复占句柄。
		res, err := resource.Admit(resource.AdmitInput{
			SourceKind: sourceKind,
			Source:     source,
			Name:       name,
			KindHint:   kindHint,
			MIME:       mimeType,
			Origin:     "channel",
			Retain:     resource.RetainEphemeral,
			Dedup:      true,
		})
		if err == nil && res.Handle != "" {
			return res.Handle, true
		}
		if err != nil {
			logger.SubWarn("LunarCore", "Napcat", "合并转发图片入库失败（%s）: %v", sourceKind, err)
		}
	}
	return "", false
}

// admitSourceKind 按来源形态选择入库通道：data URI 解码、直链下载、其余按本地文件
func admitSourceKind(source string) string {
	lower := strings.ToLower(source)
	switch {
	case strings.HasPrefix(lower, "data:"):
		return resource.SourceKindData
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return resource.SourceKindURL
	default:
		return resource.SourceKindPath
	}
}

// mediaDisplayName 推断入站媒体的展示文件名
//
// 取值顺序：显式文件名 → 文件引用 → 直链；只使用来源方声明的名字，
// 不按扩展名改写、不补造扩展名（类型判定统一交给分类层）。
func mediaDisplayName(fallbackName string, candidates ...string) string {
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate)
		if name == "" || strings.HasPrefix(name, "data:") || strings.HasPrefix(name, "base64://") {
			continue
		}
		if cut := strings.IndexAny(name, "?#"); cut >= 0 {
			name = name[:cut]
		}
		name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
		if name == "" || name == "." || name == "/" {
			continue
		}
		return name
	}
	return fallbackName
}

// resolveVideoSource 获取视频的可访问地址（优先 url，其次通过 get_file 下载）
func resolveVideoSource(videoData napcat.VideoData) string {
	if videoData.URL != "" {
		return videoData.URL
	}
	if videoData.File != "" {
		if strings.HasPrefix(videoData.File, "http://") || strings.HasPrefix(videoData.File, "https://") {
			return videoData.File
		}
		if source, err := napcat.GetVideoSource(videoData.File); err == nil && source != "" {
			return source
		}
	}
	return ""
}

// resolveAudioSource 获取语音的可访问地址
// 优先 url 直链；其次通过 get_record 转码（QQ 语音为 silk/amr 格式，由 NapCat 转为通用音频）；
// 最后退化 get_file 获取原始文件（url / 本地路径 / base64 data URI）
func resolveAudioSource(recordData napcat.RecordData) string {
	if recordData.URL != "" {
		return recordData.URL
	}
	if recordData.File == "" {
		return ""
	}
	if strings.HasPrefix(recordData.File, "http://") || strings.HasPrefix(recordData.File, "https://") {
		return recordData.File
	}
	// get_record 转码：mp3 通用格式，语音理解管线可识别
	if data, err := napcat.GetRecordSource(recordData.File); err == nil {
		if data.Base64 != "" {
			return "data:audio/mp3;base64," + data.Base64
		}
		if data.URL != "" {
			return data.URL
		}
		if data.File != "" {
			return data.File
		}
	} else {
		logger.SubError("LunarCore", "Napcat", "get_record 转码失败: %v", err)
	}
	// 退化 get_file 获取原始语音文件
	if source, err := napcat.GetVideoSource(recordData.File); err == nil && source != "" {
		return source
	}
	return ""
}

// classifySegmentBytes 把入站字节交给分类层判定类型、MIME 与扩展名
//
// 分类层需要本地文件才能读文件头，这里只做一次临时落盘；写入侧不保留任何
// 按扩展名的类型判定，也不自行补造扩展名。
func classifySegmentBytes(name string, data []byte) resource.ClassifyResult {
	declared := resource.ClassifyInput{Name: name}
	if len(data) == 0 {
		return resource.Classify(declared)
	}
	tmp, err := os.CreateTemp("", "lunar-segment-*")
	if err != nil {
		return resource.Classify(declared)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return resource.Classify(declared)
	}
	tmp.Close()
	return resource.Classify(resource.ClassifyInput{
		SourceKind: resource.SourceKindPath,
		Source:     tmp.Name(),
		LocalPath:  tmp.Name(),
		Name:       name,
	})
}
