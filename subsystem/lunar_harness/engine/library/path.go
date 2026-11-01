package library

// path.go 资源库路径校验：只允许 4 个存储根目录内的相对路径。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// localDir 内容仓库根目录（LocalDir）。
func localDir() string {
	return filepath.Clean(config.LocalDir)
}

// normalizeRel 规整相对 LocalDir 的路径（统一斜杠、去掉前缀与多余层级）。
func normalizeRel(rel string) string {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimPrefix(rel, "local_data/")
	if rel == "" || rel == "." {
		return ""
	}
	if strings.Contains(rel, "..") || strings.ContainsRune(rel, 0) {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

// rootOf 返回路径所属的存储根（不在 4 个根内返回空串）。
func rootOf(rel string) string {
	rel = normalizeRel(rel)
	if rel == "" {
		return ""
	}
	root := rel
	if before, _, ok := strings.Cut(rel, "/"); ok {
		root = before
	}
	for _, candidate := range Roots {
		if root == candidate {
			return candidate
		}
	}
	return ""
}

// resolve 把资源库相对路径解析为绝对路径；越出 LocalDir 或不在 4 个根内一律拒绝。
func resolve(rel string) (string, error) {
	rel = normalizeRel(rel)
	if rel == "" {
		return "", fmt.Errorf("路径不能为空")
	}
	if rootOf(rel) == "" {
		return "", fmt.Errorf("路径不在资源库范围内")
	}
	base := localDir()
	full := filepath.Clean(filepath.Join(base, filepath.FromSlash(rel)))
	if !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越出资源库根目录")
	}
	// 内容仓库根本身也是合法路径（用于列目录）
	if !resource.IsStoragePath(rel) {
		return "", fmt.Errorf("路径不在资源库范围内")
	}
	return full, nil
}

// resolveDir 解析目标目录（必须位于 4 个根内且已存在）。
func resolveDir(rel string) (string, error) {
	full, err := resolve(rel)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(full)
	if statErr != nil || info.IsDir() != true {
		return "", fmt.Errorf("目标目录不存在")
	}
	return full, nil
}

// absOf 返回路径对应的绝对位置（不做存在性检查）。
func absOf(rel string) string {
	return filepath.Join(localDir(), filepath.FromSlash(normalizeRel(rel)))
}

// extOf 返回小写扩展名（含点）。
func extOf(rel string) string {
	return strings.ToLower(filepath.Ext(rel))
}

// validName 新文件名只允许单段、不含路径分隔符。
func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("名称不能为空")
	}
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("名称不能包含路径分隔符")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("名称不合法")
	}
	return name, nil
}

// dirOf 返回父目录的相对路径（根目录返回根名本身）。
func dirOf(rel string) string {
	rel = normalizeRel(rel)
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if dir == "." {
		return rootOf(rel)
	}
	return dir
}

// joinRel 拼接相对路径。
func joinRel(dir, name string) string {
	return normalizeRel(filepath.ToSlash(filepath.Join(filepath.FromSlash(dir), name)))
}
