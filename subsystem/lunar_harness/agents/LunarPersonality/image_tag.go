package LunarPersonality

import (
	"net/url"
	"regexp"
	"strings"
)

// 支持的视频文件扩展名
var videoFormatsExtensions = []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv"}

var (
	tagParamRegex = regexp.MustCompile(`[?&]tag=([^&]+)`)
	imageExtRegex = regexp.MustCompile(`^\.[a-z0-9]{2,5}$`)
	dataMimeRegex = regexp.MustCompile(`^data:image/(\w+)[;,]`)
)

// guessImageExt 按来源推断图片扩展名
func guessImageExt(source string) string {
	lower := strings.ToLower(source)
	if match := dataMimeRegex.FindStringSubmatch(lower); match != nil {
		if match[1] == "jpeg" {
			return ".jpg"
		}
		return "." + match[1]
	}
	path := lower
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	ext := path[strings.LastIndex(path, "."):]
	if imageExtRegex.MatchString(ext) {
		return ext
	}
	return ".png"
}

// handleFromVisionURL 从视觉读取 URL 的 tag 参数解析句柄
func handleFromVisionURL(source string) string {
	match := tagParamRegex.FindStringSubmatch(source)
	if match == nil {
		return ""
	}
	tag := match[1]
	if decoded, err := url.QueryUnescape(tag); err == nil {
		tag = decoded
	}
	if !visionTagPattern.MatchString(tag) {
		return ""
	}
	return tag
}

func hasVideoExt(source string) bool {
	for _, ext := range videoFormatsExtensions {
		if strings.HasSuffix(source, ext) {
			return true
		}
	}
	return false
}

// sanitizeImageDesc 规整图片描述，保证可安全嵌入占位
func sanitizeImageDesc(raw string, maxChars int) string {
	text := strings.Join(strings.Fields(raw), " ")
	if text == "" {
		return ""
	}
	text = strings.TrimLeft(text, "「『\"'“”‘’")
	text = strings.TrimRight(text, "」』\"'“”‘’")
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer("[", "", "]", "", "【", "", "】", "", "〔", "", "〕", "", "（", "", "）", "", "(", "", ")", "")
	text = replacer.Replace(text)
	text = strings.TrimSpace(text)
	text = strings.TrimLeft(text, "，。、；：,.!?！？~～…-")
	text = strings.TrimRight(text, "，。、；：,.!?！？~～…-")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) > maxChars {
		return strings.TrimSpace(string(runes[:maxChars])) + "…"
	}
	return text
}

// composeImagePlaceholder 组装降级占位文本
func composeImagePlaceholder(tag, desc string) string {
	clean := sanitizeImageDesc(desc, 40)
	if clean == "" {
		return "[" + tag + "]"
	}
	return "[" + tag + "]（" + clean + "）"
}
