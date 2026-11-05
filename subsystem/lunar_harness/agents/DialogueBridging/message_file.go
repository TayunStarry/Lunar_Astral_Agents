package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/clients/napcat"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/resource"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func processFileSegment(fileData napcat.FileData) (string, bool, string) {
	fileName := fileSegmentName(fileData)

	// 未提供任何可读取标识时，仅回传名称与大小信息
	if fileData.FileID == "" && fileData.File == "" && fileData.URL == "" && fileData.Path == "" {
		return describeFile(fileName, fileData.FileSize), false, ""
	}

	data, err := readFileSegmentBytes(fileData)
	if err != nil || len(data) == 0 {
		logger.SubError("LunarCore", "Napcat", "下载文件失败: %v", err)
		return describeFile(fileName, fileData.FileSize), false, ""
	}

	// 图片文件 → 作为多媒体内容处理（扩展名缺失时按文件头嗅探）
	if mimeType := imageMIME(fileName); mimeType != "" {
		return "", true, "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	if mimeType := sniffImageMIME(data); mimeType != "" {
		return "", true, "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}

	// 视频文件 → 先落到 multimedia/，再以 @U 链接句柄回引（不再压成 @V 归档副本）
	ext := strings.ToLower(filepath.Ext(fileName))
	if resource.KindOfExt(ext) == resource.KindVideo {
		if url := saveVideoFileURL(fileName, data); url != "" {
			if res, err := resource.AdmitURL(resource.URLAdmitInput{
				URL:      url,
				KindHint: resource.KindVideo,
				Name:     fileName,
				Title:    fileName,
				Origin:   resource.OriginChannel,
			}); err == nil && res.Handle != "" {
				return "[" + res.Handle + "] " + fileName + " ", false, ""
			}
		}
		return fileName + " ", false, ""
	}

	// 其余文件（文本 / 代码 / 表格 / 二进制 / 未知类型）一律作文档句柄
	// （文本/表格可在思考链中按句柄读取；解析不了的文件仍保留句柄与文件名）
	kindHint := resource.KindOfExt(ext)
	if kindHint == "" {
		kindHint = resource.KindDocument
	}
	res, err := resource.Admit(resource.AdmitInput{
		SourceKind: resource.SourceKindBytes,
		Data:       data,
		Name:       fileName,
		KindHint:   kindHint,
		Origin:     resource.OriginChannel,
		Retain:     resource.RetainNormal,
	})
	if err != nil || res.Handle == "" {
		logger.SubError("LunarCore", "Napcat", "文件入库失败: %v", err)
		return describeFile(fileName, fileData.FileSize), false, ""
	}
	name := res.Name
	if name == "" {
		name = fileName
	}
	return "[" + res.Handle + "] " + name + " ", false, ""
}

// fileSegmentName 取文件段的展示文件名（显式名优先，其次引用路径基名）
func fileSegmentName(fileData napcat.FileData) string {
	candidates := []string{fileData.FileName, fileData.Name, fileData.File}
	if fileData.Path != "" {
		candidates = append(candidates, napcat.FileURLPath(fileData.Path))
	}
	return mediaDisplayName("", "未命名文件", candidates...)
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

// imageMIME 通过扩展名判断并返回图片 MIME 类型，非图片返回空字符串
func imageMIME(fileName string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// sniffImageMIME 通过文件头识别常见图片格式，非图片返回空字符串
func sniffImageMIME(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png"
	case len(data) >= 3 && bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case len(data) >= 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))):
		return "image/gif"
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	case len(data) >= 2 && bytes.HasPrefix(data, []byte("BM")):
		return "image/bmp"
	default:
		return ""
	}
}

// dataURIExt 从 data:image/... URI 的 MIME 推断扩展名（未知时回退 .png）
func dataURIExt(dataURI string) string {
	meta := dataURI
	if end := strings.IndexAny(meta, ";,"); end >= 0 {
		meta = meta[:end]
	}
	switch strings.ToLower(strings.TrimPrefix(meta, "data:")) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	default:
		return ".png"
	}
}

// forwardImageName 为转发图片生成带图片扩展名的建议文件名。
//
// QQ CDN 直链经常没有扩展名；如果没有扩展名，归档层会按文档处理。
// 这里未知格式一律先用 .jpg，归档层随后会按文件头嗅探把 GIF/WebP 等纠正过来。
func forwardImageName(source string) string {
	if strings.HasPrefix(source, "data:") {
		return "forward" + dataURIExt(source)
	}
	pathPart := source
	if cut := strings.IndexAny(pathPart, "?#"); cut >= 0 {
		pathPart = pathPart[:cut]
	}
	switch ext := strings.ToLower(filepath.Ext(pathPart)); ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return "forward" + ext
	default:
		return "forward.jpg"
	}
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

// processForwardSegment 展开合并转发消息（含嵌套转发递归），拼接为文本描述
// 子消息中的图片注册进共享图片缓存，正文内嵌 [图片.00x] 占位标签（按标签取回，不再注入多模态项）
