package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// processFileSegment 处理文件消息段，返回 (文本描述, 信封类型, 信封载荷)。
//
// 文件通道可以承载图片：类型以字节嗅探为准（QQ 常用 .jpg / .png 文件名存放
// GIF 数据），嗅探失败才退回来源方声明的文件名扩展名；其余文件仍落资源归档
// 并交出句柄文本，避免把大文件 base64 塞进消息体。
func processFileSegment(groupID int64, fileData napcat.FileData) (string, string, any) {
	fileName := fileSegmentName(fileData)

	// 未提供任何可读取标识时，仅回传名称与大小信息
	if fileData.FileID == "" && fileData.File == "" && fileData.URL == "" && fileData.Path == "" {
		return describeFile(fileName, fileData.FileSize), "", nil
	}

	data, err := readFileSegmentBytes(fileData)
	if err != nil || len(data) == 0 {
		logger.SubError("LunarCore", "Napcat", "下载文件失败: %v", err)
		return describeFile(fileName, fileData.FileSize), "", nil
	}

	// 图片 / 动图：先按字节嗅探真实类型；嗅探失败才退回文件名扩展名声明
	mimeType, animated := sniffImageBytes(data)
	if mimeType == "" {
		if extMIME := mediaobject.MIMETypeOfExt(fileName); strings.HasPrefix(extMIME, "image/") {
			logger.SubWarn("LunarCore", "Napcat", "文件图片嗅探失败，按文件名扩展名声明: name=%s mime=%s", fileName, extMIME)
			mimeType, animated = extMIME, mediaobject.IsAnimatedImageMIME(extMIME)
		}
	}
	if mimeType != "" {
		dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		envelope, payload := buildFileImageEnvelope(groupID, dataURI, fileName, mimeType, animated)
		return fileName + " ", envelope, payload
	}

	// 其余一律作文档句柄（文本/代码/表格可在思考链中按句柄读取）
	res, err := resource.Admit(resource.AdmitInput{
		SourceKind: resource.SourceKindBytes,
		Data:       data,
		Name:       fileName,
		Origin:     resource.OriginChannel,
		Retain:     resource.RetainNormal,
	})
	if err != nil || res.Handle == "" {
		logger.SubError("LunarCore", "Napcat", "文件入库失败: %v", err)
		return describeFile(fileName, fileData.FileSize), "", nil
	}
	name := res.Name
	if name == "" {
		name = fileName
	}
	return "[" + res.Handle + "] " + name + " ", "", nil
}

// fileSegmentName 取文件段的展示文件名（显式名优先，其次引用路径基名）
func fileSegmentName(fileData napcat.FileData) string {
	candidates := []string{fileData.FileName, fileData.Name, fileData.File}
	if fileData.Path != "" {
		candidates = append(candidates, napcat.FileURLPath(fileData.Path))
	}
	return mediaDisplayName("未命名文件", candidates...)
}

// readFileSegmentBytes 按可用性依次尝试文件段的三种读取来源
func readFileSegmentBytes(fileData napcat.FileData) ([]byte, error) {
	if path := strings.TrimSpace(fileData.Path); path != "" {
		if data, err := napcat.FetchFileBytes(path); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	if fileData.FileID != "" || fileData.File != "" {
		if data, err := napcat.GetFileContent(fileData.FileID, fileData.File); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	if url := strings.TrimSpace(fileData.URL); url != "" {
		return napcat.FetchFileBytes(url)
	}
	return nil, fmt.Errorf("文件没有可用的读取来源")
}

// describeFile 构建文件的名称与大小信息描述
//
// 句柄不可得时退化为文件名（附带大小），不再写 "[文件]" 这类无信息占位。
func describeFile(fileName string, sizeRaw json.RawMessage) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = "未命名文件"
	}
	var sb strings.Builder
	sb.WriteString(name)
	if size := fileSizeText(sizeRaw); size != "" {
		sb.WriteString("（")
		sb.WriteString(size)
		sb.WriteString(" 字节）")
	}
	sb.WriteString(" ")
	return sb.String()
}

// fileSizeText 将 file_size 字段（可能为数字或字符串）解析为字符串
func fileSizeText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return ""
}

// sniffImageBytes 通过文件头魔数嗅探真实图片类型与动画能力，非图片返回空 MIME。
//
// QQ 常把 GIF 数据存成 .jpg / .png 文件名，信封声明必须以字节为准：
// PNG 复用 persistence.SniffImageFormat 的 acTL 判定识别 APNG，WebP 扫 ANIM 块识别动态 WebP，GIF 统一按动图声明。
func sniffImageBytes(data []byte) (string, bool) {
	switch {
	case len(data) >= 8 && bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		_, mimeType := persistence.SniffImageFormat(data)
		return mimeType, mimeType == "image/apng"
	case len(data) >= 3 && bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg", false
	case len(data) >= 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))):
		return "image/gif", true
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", persistence.IsAnimatedWebP(data)
	case len(data) >= 2 && bytes.HasPrefix(data, []byte("BM")):
		return "image/bmp", false
	default:
		return "", false
	}
}

// sniffImageMIME 只取嗅探出的 MIME，供只关心类型的调用点复用
func sniffImageMIME(data []byte) string {
	mimeType, _ := sniffImageBytes(data)
	return mimeType
}

// decodeDataURIBytes 解码 data URI 的 base64 载荷；非 base64 data URI 返回空
func decodeDataURIBytes(ref string) []byte {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(strings.ToLower(ref), "data:") {
		return nil
	}
	comma := strings.IndexByte(ref, ',')
	if comma < 0 || !strings.Contains(strings.ToLower(ref[:comma]), ";base64") {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(ref[comma+1:])
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// extractJsonCardTitle 从 JSON 卡片数据中提取标题（小程序 / 分享卡片 / 邀请等）
func extractJsonCardTitle(data string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(data), &payload) != nil {
		return ""
	}
	if title, ok := payload["prompt"].(string); ok && title != "" {
		return title
	}
	if meta, ok := payload["meta"].(map[string]any); ok {
		for _, v := range meta {
			if m, ok := v.(map[string]any); ok {
				if title, ok := m["title"].(string); ok && title != "" {
					return title
				}
			}
		}
	}
	if title, ok := payload["title"].(string); ok && title != "" {
		return title
	}
	return ""
}

// extractJsonCardLink 从 JSON 卡片数据中提取可下发的 http/https 链接。
//
// QQ 卡片把链接放在不同层：顶层 url，或 meta.<业务>.jumpUrl / qqdocurl / url
// （news 为分享卡片，detail_1 为公众号文章）。按固定顺序扫描，保证结果稳定。
func extractJsonCardLink(data string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(data), &payload) != nil {
		return ""
	}
	if link, ok := payload["url"].(string); ok && isHTTPLink(link) {
		return strings.TrimSpace(link)
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item, ok := meta[key].(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"jumpUrl", "qqdocurl", "url"} {
			if link, ok := item[field].(string); ok && isHTTPLink(link) {
				return strings.TrimSpace(link)
			}
		}
	}
	return ""
}

// processForwardSegment 展开合并转发消息（含嵌套转发递归），拼接为文本描述
// 子消息中的图片注册进共享图片缓存，正文内嵌 [图片.00x] 占位标签（按标签取回，不再注入多模态项）
