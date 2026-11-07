package resource

// classify_rules.go 分类器的静态规则：MIME / 扩展名归一
//
// 入站信封只使用「声明 MIME」；扩展名与兜底规则仅供存储层归位（DetectStorageKind）。

import (
	"strings"
)

// normalizeExt 归一扩展名：小写、带点、只保留字母数字
func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if cut := strings.IndexAny(ext, "?#"); cut >= 0 {
		ext = ext[:cut]
	}
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		return ""
	}
	for _, r := range ext {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return "." + ext
}

// normalizeMIME 去参数并小写
func normalizeMIME(raw string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(raw, ";", 2)[0]))
}

// kindFromMIME 由 MIME 声明的类型族推断类型码与首选扩展名
func kindFromMIME(raw string) (string, string) {
	mimeType := normalizeMIME(raw)
	switch mimeType {
	case "", "application/octet-stream", "binary/octet-stream", "application/unknown":
		return "", ""
	case "image/gif", "image/apng":
		return KindAnimated, ".gif"
	case "image/jpeg":
		return KindImage, ".jpg"
	case "image/png":
		return KindImage, ".png"
	case "image/webp":
		return KindImage, ".webp"
	case "image/svg+xml":
		return KindImage, ".svg"
	case "image/bmp":
		return KindImage, ".bmp"
	case "image/tiff":
		return KindImage, ".tiff"
	case "image/avif":
		return KindImage, ".avif"
	case "image/heic", "image/heif":
		return KindImage, ".heic"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return KindImage, ".ico"
	case "video/mp4":
		return KindVideo, ".mp4"
	case "video/webm":
		return KindVideo, ".webm"
	case "video/quicktime":
		return KindVideo, ".mov"
	case "video/x-msvideo":
		return KindVideo, ".avi"
	case "video/x-matroska":
		return KindVideo, ".mkv"
	case "video/x-flv":
		return KindVideo, ".flv"
	case "video/mpeg":
		return KindVideo, ".mpg"
	case "audio/mpeg", "audio/mp3":
		return KindAudio, ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return KindAudio, ".wav"
	case "audio/flac", "audio/x-flac":
		return KindAudio, ".flac"
	case "audio/ogg":
		return KindAudio, ".ogg"
	case "audio/mp4", "audio/x-m4a":
		return KindAudio, ".m4a"
	case "audio/aac":
		return KindAudio, ".aac"
	case "audio/opus":
		return KindAudio, ".opus"
	case "text/html", "application/xhtml+xml":
		return KindURL, ".html"
	case "application/pdf":
		return KindDocument, ".pdf"
	case "application/zip":
		return KindDocument, ".zip"
	case "application/json":
		return KindDocument, ".json"
	case "text/plain":
		return KindDocument, ".txt"
	}
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return KindImage, ""
	case strings.HasPrefix(mimeType, "video/"):
		return KindVideo, ""
	case strings.HasPrefix(mimeType, "audio/"):
		return KindAudio, ""
	case strings.HasPrefix(mimeType, "text/"), strings.HasSuffix(mimeType, "+xml"):
		return KindDocument, ""
	}
	return "", ""
}

// mimeForKind 按类型码与扩展名给出规范 MIME（链接类统一用 text/uri-list）
func mimeForKind(kind, ext string) string {
	if kind == KindURL {
		return URLMIME
	}
	return resolveMIME("", ext)
}

// isMediaKind 是否为媒体类类型码（图片 / 动图 / 视频 / 音频）
func isMediaKind(kind string) bool {
	switch kind {
	case KindImage, KindAnimated, KindVideo, KindAudio:
		return true
	}
	return false
}
