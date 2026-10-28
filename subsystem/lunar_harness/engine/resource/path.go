package resource

// path.go 相对 LocalDir 的路径规整与 MIME 推断。

import (
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// normalizeRel 规整相对 LocalDir 的路径为斜杠形式
func normalizeRel(rel string) string {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "/")
	if rel == "." {
		return ""
	}
	return rel
}

// relativeToLocalDir 求绝对路径相对 LocalDir 的斜杠形式路径（不在其下时返回 false）
func relativeToLocalDir(abs string) (string, bool) {
	localDir := filepath.Clean(config.LocalDir)
	rel, err := filepath.Rel(localDir, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// fileExists 判断路径是否为存在的普通文件
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// stripBOM 去除 UTF-8 BOM
func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// firstNonEmpty 返回首个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveMIME 解析 MIME：优先调用方建议值，其次扩展名白名单（persistence 的预览白名单），
// 最后按类型码兜底。
func resolveMIME(hint, ext string) string {
	if hint != "" {
		return hint
	}
	if mimeType, ok := persistence.PreviewMIME(strings.ToLower(ext)); ok {
		return mimeType
	}
	switch KindOfExt(ext) {
	case KindImage:
		return "image/" + strings.TrimPrefix(strings.ToLower(ext), ".")
	case KindVideo:
		return "video/" + strings.TrimPrefix(strings.ToLower(ext), ".")
	case KindAudio:
		return "audio/" + strings.TrimPrefix(strings.ToLower(ext), ".")
	case KindDocument:
		return "text/plain"
	}
	return "application/octet-stream"
}
