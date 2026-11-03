package resourcehub

// render.go 单个句柄 / 路径的取回与解读。
//
// 顺序严格遵循规范：已有解析文本直接返回；否则按类型分流到
// 视觉描述 / 音频转写 / 网页现场读取 / B 站直驱 / 归档解析 / 文档原文。
//
// 所有跨包能力都经 Hooks 注入，engine 不 import 任何 agents 包。

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// RenderHandle 读取单个句柄或文件名 / 路径，返回模型可见的资源块。
//
// 输入不是句柄时，先经统一定位器解析并登记为句柄，再走同一套渲染逻辑。
// 返回块文本、实际句柄与错误；错误发生时调用方可把它归入「无法取回」列表。
func RenderHandle(input, ask string) (string, string, error) {
	handle := parseHandleText(input)
	if handle == "" {
		match, err := ResolveToHandle(input, nil)
		if err != nil {
			return "", "", err
		}
		handle = match.Handle
	}
	entry, found := resource.Lookup(handle)
	if !found {
		return "", handle, fmt.Errorf("%s %s", handle, handleInvalidNotice)
	}
	normalized := entry.Handle
	if normalized == "" {
		normalized = handle
	}
	if entry.State == resource.StateLost {
		_ = resource.Delete(normalized)
		return resourceBlockHeader(normalized, entry) + lostFileNotice, normalized, nil
	}
	hooks := currentHooks()
	// 压缩包 / GGUF 等归档类实体：不按文本取回，交给归档解析
	if hooks.IsArchiveExt != nil && hooks.IsArchiveExt(entryExt(entry)) {
		return archiveBlock(normalized, entry, ask, hooks), normalized, nil
	}
	// 解析文本优先（文件类直接读原文，不在此短路）
	if entry.Kind != resource.KindDocument &&
		entry.ParseState == resource.ParseReady && strings.TrimSpace(entry.ParseText) != "" {
		return resourceBlockHeader(normalized, entry) + entry.ParseText, normalized, nil
	}
	switch entry.Kind {
	case resource.KindImage, resource.KindAnimated:
		return imageBlock(normalized, entry, ask, hooks), normalized, nil
	case resource.KindAudio:
		return audioBlock(normalized, entry), normalized, nil
	case resource.KindVideo:
		return videoBlock(normalized, entry, ask, hooks), normalized, nil
	case resource.KindURL:
		return webBlock(normalized, entry, ask, hooks), normalized, nil
	default:
		return documentBlock(normalized, entry, ask, hooks), normalized, nil
	}
}

// imageBlock 图片 / 动图：本地已有副本直接解读；远程直链先下载再解读
func imageBlock(handle string, entry resource.Resource, ask string, hooks Hooks) string {
	originalURL := entry.URL
	if entry.BlobPath == "" && resource.IsRemoteURL(entry.URL) {
		if admitted := admitMedia(entry.URL, mediaName(entry.URL, ".gif"), entry.Origin, entry.Kind); admitted != nil && admitted.Handle != "" {
			if local, ok := resource.Lookup(admitted.Handle); ok {
				handle, entry = admitted.Handle, local
			}
		}
	}
	desc := ""
	if hooks.DescribeMedia != nil {
		desc = hooks.DescribeMedia(handle, ask)
	}
	if desc == "" {
		if _, ok := resource.Lookup(handle); !ok {
			return resourceBlockHeader(handle, entry) + lostFileNotice
		}
		desc = "（暂无描述，且本体不可直接解读）"
	}
	// 远程来源的解析文本同时写回原 URL，保证同一链接下次直接命中
	if originalURL != "" && resource.IsRemoteURL(originalURL) {
		_ = resource.SetParseText(originalURL, desc)
	}
	return resourceBlockHeader(handle, entry) + desc
}

// audioBlock 音频：读取接入阶段写入的转写描述（无实体音频可解码）
func audioBlock(handle string, entry resource.Resource) string {
	text := resource.ParseTextOfResource(handle)
	if strings.TrimSpace(text) == "" {
		text = resource.ASRFallbackText
	}
	return resourceBlockHeader(handle, entry) + text
}

// videoBlock 视频：B 站走直驱印象；本地或远程直链走观看管线
func videoBlock(handle string, entry resource.Resource, ask string, hooks Hooks) string {
	rawURL := entry.URL
	if resource.BilibiliBvid(rawURL) != "" {
		impression, err := bilibiliImpression(rawURL, ask, hooks)
		if err != nil {
			return resourceBlockHeader(handle, entry) + "视频印象获取失败：" + err.Error()
		}
		if strings.TrimSpace(impression) != "" {
			_ = resource.SetParseText(rawURL, impression)
		}
		return resourceBlockHeader(handle, entry) + impression
	}
	source := rawURL
	remote := entry.BlobPath == "" || !resource.IsLocalURL(rawURL)
	if remote {
		admitted := admitMedia(rawURL, mediaName(rawURL, ".mp4"), entry.Origin, resource.KindVideo)
		if admitted == nil || admitted.Handle == "" {
			return resourceBlockHeader(handle, entry) + "视频读取失败：本地副本未能下载落盘"
		}
		source = admitted.URL
	}
	desc, err := describeVideo(source, ask, hooks)
	if err != nil {
		// 远程地址失效且本地无副本：重新下载一次
		if remote && hooks.IsVideoExpired != nil && hooks.IsVideoExpired(err) {
			if admitted := admitMedia(rawURL, mediaName(rawURL, ".mp4"), entry.Origin, resource.KindVideo); admitted != nil {
				if retry, retryErr := describeVideo(admitted.URL, ask, hooks); retryErr == nil {
					return resourceBlockHeader(handle, entry) + retry
				}
			}
			return resourceBlockHeader(handle, entry) + "这段视频的远程地址已经失效，本地也没有可复用的副本；请对方重新发送一次视频。"
		}
		return resourceBlockHeader(handle, entry) + "视频读取失败：" + err.Error()
	}
	if desc == "" {
		desc = "（视频暂无可用描述）"
	}
	if entry.URL != "" {
		_ = resource.SetParseText(entry.URL, desc)
	}
	return resourceBlockHeader(handle, entry) + desc
}

// webBlock 网页：本地已有视频 / 动图副本走观看管线；否则网页现场读取
func webBlock(handle string, entry resource.Resource, ask string, hooks Hooks) string {
	rawURL := entry.URL
	if resource.BilibiliBvid(rawURL) != "" {
		impression, err := bilibiliImpression(rawURL, ask, hooks)
		if err != nil {
			return resourceBlockHeader(handle, entry) + "视频印象获取失败：" + err.Error()
		}
		if strings.TrimSpace(impression) != "" {
			_ = resource.SetParseText(rawURL, impression)
		}
		return resourceBlockHeader(handle, entry) + impression
	}
	ext := strings.ToLower(filepath.Ext(pathPartOf(rawURL)))
	if resource.KindOfExt(ext) == resource.KindVideo || (resource.MayBeAnimated(ext) && resource.KindOfExt(ext) == resource.KindImage) {
		admitted := admitMedia(rawURL, mediaName(rawURL, ext), entry.Origin, "")
		if admitted != nil && admitted.URL != "" {
			desc, err := describeVideo(admitted.URL, ask, hooks)
			if err == nil && strings.TrimSpace(desc) != "" {
				_ = resource.SetParseText(rawURL, desc)
				return resourceBlockHeader(handle, entry) + desc
			}
		}
	}
	if hooks.ReadPage == nil {
		return resourceBlockHeader(handle, entry) + "（当前没有可用的网页读取能力）"
	}
	title, summary, err := hooks.ReadPage(rawURL, ask)
	if err != nil {
		return resourceBlockHeader(handle, entry) + "这个链接现在打不开：" + err.Error()
	}
	if strings.TrimSpace(summary) == "" {
		return resourceBlockHeader(handle, entry) + "（这个链接没有读取出可用内容）"
	}
	_ = resource.SetParseText(rawURL, summary)
	if strings.TrimSpace(title) != "" {
		_, _ = resource.AdmitURL(resource.URLAdmitInput{URL: rawURL, Title: title, Origin: entry.Origin})
	}
	return resourceBlockHeader(handle, entry) + summary
}

// documentBlock 文件：直接取原文；带 ask 时交给文献解析智能体探查
func documentBlock(handle string, entry resource.Resource, ask string, hooks Hooks) string {
	if ask != "" {
		answer := ""
		if hooks.InspectText != nil {
			answer = hooks.InspectText(handle, ask)
		}
		if answer == "" {
			return resourceBlockHeader(handle, entry) + "探查失败：文献解析没有给出答复，可去掉 ask 直接取原文"
		}
		return resourceBlockHeader(handle, entry) + answer
	}
	result, err := resource.Read(handle, documentReadBytes)
	if err != nil {
		return resourceBlockHeader(handle, entry) + "取回失败：" + err.Error()
	}
	if result.Lost {
		_ = resource.Delete(handle)
		return resourceBlockHeader(handle, entry) + lostFileNotice
	}
	rangeNote := ""
	if result.LineStart > 0 && result.LineEnd > 0 {
		rangeNote = fmt.Sprintf("｜第 %d-%d 行", result.LineStart, result.LineEnd)
	}
	sectionNote := ""
	if result.SectionTitle != "" {
		sectionNote = "｜" + result.SectionTitle
	}
	truncated := ""
	if result.Truncated {
		truncated = "\n（内容较长已截断；可带 |L起-止 或 |§n 定向读取）"
	}
	return fmt.Sprintf("【%s%s%s｜共 %d 行】\n%s%s", handle, sectionNote, rangeNote, result.TotalLines, result.Content, truncated)
}

// archiveBlock 压缩包 / GGUF：交给归档解析智能体做确定性解析
func archiveBlock(handle string, entry resource.Resource, ask string, hooks Hooks) string {
	if strings.TrimSpace(entry.BlobPath) == "" {
		return resourceBlockHeader(handle, entry) + "（该归档实体没有可读取的本地副本）"
	}
	if hooks.InspectArchive == nil {
		return resourceBlockHeader(handle, entry) + "（当前没有可用的归档解析能力）"
	}
	text, err := hooks.InspectArchive(entry.BlobPath, ask)
	if err != nil {
		return resourceBlockHeader(handle, entry) + "解析失败：" + err.Error()
	}
	return resourceBlockHeader(handle, entry) + text
}

// bilibiliImpression 调用注入的 B 站直驱能力
func bilibiliImpression(rawURL, ask string, hooks Hooks) (string, error) {
	if hooks.BilibiliImpression == nil {
		return "", errors.New("当前没有可用的视频印象能力")
	}
	return hooks.BilibiliImpression(rawURL, ask)
}

// describeVideo 调用注入的视频描述能力
func describeVideo(rawURL, ask string, hooks Hooks) (string, error) {
	if hooks.DescribeVideo == nil {
		return "", errors.New("当前没有可用的视频解读能力")
	}
	return hooks.DescribeVideo(rawURL, ask)
}

// admitMedia 把媒体收进资源目录（失败时静默返回 nil，由调用方降级）
func admitMedia(source, name, origin, kindHint string) *resource.AdmitResult {
	if source == "" {
		return nil
	}
	lower := strings.ToLower(source)
	sourceKind := resource.SourceKindPath
	if strings.HasPrefix(lower, "data:") {
		sourceKind = resource.SourceKindData
	} else if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		sourceKind = resource.SourceKindURL
	}
	result, err := resource.Admit(resource.AdmitInput{
		SourceKind: sourceKind,
		Source:     source,
		Name:       name,
		KindHint:   kindHint,
		Origin:     origin,
	})
	if err != nil {
		return nil
	}
	return &result
}

// resourceBlockHeader 统一的资源块标题
func resourceBlockHeader(handle string, entry resource.Resource) string {
	kind := ""
	if parsed, ok := resource.ParseHandle(handle); ok {
		kind = parsed.Kind
	}
	title := strings.TrimSpace(entry.Title)
	if title == "" {
		title = strings.TrimSpace(entry.Name)
	}
	if title == "" {
		title = handle
	}
	return "【" + handle + "｜" + kindLabel(kind) + "｜" + title + "】\n"
}
