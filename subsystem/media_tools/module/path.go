// 路径解析：支持绝对路径与相对 LocalDir 的路径
package module

import (
	"LunarSubsystem/GeneralConfig"
	"path/filepath"
	"strings"
)

// ResolvePath 将请求中的路径解析为本地绝对路径
// 绝对路径直接返回；相对路径以 LocalDir 为基准拼接（如文件管理器传递的相对路径）
func ResolvePath(path string) string {
	if path == "" {
		return ""
	}
	// 统一分隔符
	normalized := filepath.FromSlash(path)
	if filepath.IsAbs(normalized) {
		return filepath.Clean(normalized)
	}
	return filepath.Clean(filepath.Join(*GeneralConfig.LocalDir, normalized))
}

// IsWithinLocalDir 校验目标路径是否位于 LocalDir 内，防止目录遍历
func IsWithinLocalDir(fullPath string) bool {
	cleaned := filepath.Clean(fullPath)
	local := filepath.Clean(*GeneralConfig.LocalDir)
	return cleaned == local || strings.HasPrefix(cleaned, local+string(filepath.Separator))
}
