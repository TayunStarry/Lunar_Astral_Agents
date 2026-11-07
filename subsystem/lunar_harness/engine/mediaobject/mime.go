package mediaobject

import (
	"path/filepath"
	"strings"
)

// MIMETypeOfExt 按来源方声明的文件名/扩展名补全 MIME；无法识别时返回空串。
// 仅使用调用方提供的声明信息，不读取文件内容。
func MIMETypeOfExt(nameOrExt string) string {
	ext := strings.ToLower(strings.TrimPrefix(nameOrExt, "."))
	if strings.Contains(ext, ".") {
		ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(ext), "."))
	}
	return mimeByExtension[ext]
}

// MIMETypeOfDataURI 取 data URI 头部声明的 MIME（如 data:image/png;base64,...）
func MIMETypeOfDataURI(ref string) string {
	if !strings.HasPrefix(ref, "data:") {
		return ""
	}
	header := ref[len("data:"):]
	if idx := strings.IndexByte(header, ';'); idx >= 0 {
		header = header[:idx]
	} else if idx = strings.IndexByte(header, ','); idx >= 0 {
		header = header[:idx]
	}
	return strings.TrimSpace(header)
}

// IsAnimatedImageMIME 按来源方声明的 MIME 判断是否为动图
func IsAnimatedImageMIME(mimeType string) bool {
	return animatedImageMIMEs[strings.ToLower(strings.TrimSpace(mimeType))]
}
