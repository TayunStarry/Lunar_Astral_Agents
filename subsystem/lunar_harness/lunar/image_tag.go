package lunar

import (
	"net/url"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
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

// attachImageTags 入队赋签：为消息中的每个图片项入库，并把来源改写为视觉读取形态
func attachImageTags(message *PostMessage) {
	if message == nil {
		return
	}
	items := asItems(message.Content)
	if len(items) == 0 {
		return
	}
	for i := range items {
		if items[i].Type != ContentImageURL || items[i].ImageURL == nil {
			continue
		}
		source := items[i].ImageURL.URL
		if source == "" {
			continue
		}
		lower := strings.ToLower(source)
		if strings.HasPrefix(lower, "data:video/") || hasVideoExt(lower) {
			continue
		}
		if strings.Contains(lower, visionReadPath) {
			continue
		}
		admitted := admitImageDataURI(source, "image"+guessImageExt(source))
		if admitted == nil || admitted.Handle == "" || admitted.BlobPath == "" {
			logger.Error(lunarCoreModule, "[图片标签] 入库失败，该图片已从思维链剔除: %s", truncateRunes(source, 60))
			items[i] = ContentItem{Type: ContentText, Text: "[一张图片入库失败，已忽略]"}
			continue
		}
		items[i].ImageURL = &URLPayload{URL: visionReference(admitted.Handle, admitted.BlobPath)}
	}
	message.Content = items
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
