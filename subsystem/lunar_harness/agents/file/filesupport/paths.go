package filesupport

// paths.go 路径解析：句柄、/document 前缀、相对路径统一落到 LocalDir 下的规范路径。

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// Resolve 把句柄或路径转成相对 LocalDir 的规范路径与绝对路径。
// root 非空时，新路径会落到 root 下；已明确在 documents/ 或 unknown/ 下的路径保持不变。
func Resolve(raw, root string) (rel, abs string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("path 不能为空（请把目标句柄或文件路径填入 path）")
	}
	if strings.HasPrefix(raw, "@") {
		entry, resolveErr := resource.Resolve(raw)
		if resolveErr != nil {
			return "", "", fmt.Errorf("解析句柄 %s 失败：%w", raw, resolveErr)
		}
		rel = entry.BlobPath
	} else {
		rel = strings.ReplaceAll(raw, "\\", "/")
		rel = strings.TrimPrefix(rel, "/")
		rel = strings.TrimPrefix(rel, "document/")
		if strings.HasPrefix(rel, "documents/") || strings.HasPrefix(rel, "unknown/") {
			// 已显式限定根目录，不再追加。
		} else if root = strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/"); root != "" {
			rel = path.Join(root, rel)
		} else {
			rel = path.Join("documents", rel)
		}
	}
	rel = path.Clean(rel)
	if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", "", fmt.Errorf("非法路径：%s", raw)
	}
	if root = strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/"); root != "" {
		if rel != root && !strings.HasPrefix(rel, root+"/") && !strings.HasPrefix(rel, "unknown/") {
			return "", "", fmt.Errorf("路径 %s 不在允许的输出根 %s 下", rel, root)
		}
	}
	abs = filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	cleanAbs := filepath.Clean(abs)
	baseAbs := filepath.Clean(config.LocalDir)
	if cleanAbs != baseAbs && !strings.HasPrefix(cleanAbs, baseAbs+string(filepath.Separator)) {
		return "", "", fmt.Errorf("路径越界：%s", rel)
	}
	return rel, cleanAbs, nil
}

// Abs 返回规范路径对应的绝对路径。
func Abs(rel string) string {
	return filepath.Join(config.LocalDir, filepath.FromSlash(rel))
}
