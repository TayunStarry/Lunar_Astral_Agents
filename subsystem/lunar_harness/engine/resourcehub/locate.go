package resourcehub

// locate.go 统一文件定位：句柄，或完整 / 相对 / 纯文件名路径。
//
// 定位顺序：句柄 → 绝对路径 → 精确路径 → 索引模糊匹配；
// 命中多个文件时返回 AmbiguousError，附带候选路径供调用方选择。
//
// 这是句柄体系与「人给的文件名 / 路径」之间的唯一转换层：
// 上层默认使用句柄，只有旧入口或用户输入才会走文件名 / 路径。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// Resolve 定位句柄或路径，roots 为相对 LocalDir 的搜索根（空 = 全部存储根）
func Resolve(input string, roots []string) (Match, error) {
	raw := normalizeInput(input)
	if raw == "" {
		return Match{}, fmt.Errorf("路径不能为空")
	}
	if strings.HasPrefix(raw, "@") {
		return resolveHandle(raw)
	}
	if strings.Contains(raw, "..") {
		return Match{}, fmt.Errorf("路径不允许包含 ..")
	}
	if filepath.IsAbs(filepath.FromSlash(raw)) {
		return resolveAbsolute(raw)
	}
	rel := raw
	switch {
	case rel == "/document":
		rel = ""
	case strings.HasPrefix(rel, "/document/"):
		rel = strings.TrimPrefix(rel, "/document/")
	case strings.HasPrefix(rel, "/"):
		return Match{}, fmt.Errorf("请使用相对路径或 /document/ 前缀")
	}
	roots = normalizedRoots(roots)
	if !strings.Contains(rel, "/") {
		return resolveByBasename(input, rel, roots)
	}
	if candidates := exactMatches(rel, roots); len(candidates) > 0 {
		if len(candidates) > 1 {
			return Match{}, &AmbiguousError{Input: input, Candidates: candidates}
		}
		return matchOf(candidates[0]), nil
	}
	return resolveFuzzy(input, rel, roots)
}

// ResolveToHandle 定位并登记资源句柄（供只接受句柄的智能体入口使用）
func ResolveToHandle(input string, roots []string) (Match, error) {
	match, err := Resolve(input, roots)
	if err != nil {
		return Match{}, err
	}
	if match.Handle != "" {
		return match, nil
	}
	_, handle, regErr := resource.RegisterPath(match.Rel)
	if regErr != nil || handle == "" {
		return Match{}, fmt.Errorf("无法登记资源句柄：%s", match.Rel)
	}
	match.Handle = handle
	return match, nil
}

// resolveHandle 句柄 → 本地实体
func resolveHandle(raw string) (Match, error) {
	res, err := resource.Resolve(raw)
	if err != nil {
		return Match{}, err
	}
	if res.BlobPath == "" {
		return Match{}, fmt.Errorf("%s 没有本地实体", res.Handle)
	}
	match := matchOf(res.BlobPath)
	if info, err := os.Stat(match.Abs); err != nil || info.IsDir() {
		return Match{}, fmt.Errorf("%s 的本体已丢失：%s", res.Handle, res.BlobPath)
	}
	match.Handle = res.Handle
	return match, nil
}

// resolveAbsolute 绝对路径必须落在 LocalDir 内
func resolveAbsolute(raw string) (Match, error) {
	abs := filepath.Clean(filepath.FromSlash(raw))
	rel, err := filepath.Rel(filepath.Clean(config.LocalDir), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return Match{}, fmt.Errorf("路径越出本地目录范围: %s", raw)
	}
	match := matchOf(filepath.ToSlash(rel))
	if info, err := os.Stat(match.Abs); err != nil || info.IsDir() {
		return Match{}, fmt.Errorf("文件不存在：%s", match.Rel)
	}
	return match, nil
}

// resolveByBasename 纯文件名：索引里按文件名（以及去扩展名）查找
func resolveByBasename(input, base string, roots []string) (Match, error) {
	candidates := fuzzyByBasename(strings.ToLower(base), roots)
	switch len(candidates) {
	case 0:
		return Match{}, fmt.Errorf("未找到文件：%s", input)
	case 1:
		return matchOf(candidates[0]), nil
	default:
		return Match{}, &AmbiguousError{Input: input, Candidates: candidates}
	}
}

// resolveFuzzy 带目录的相对路径：精确未命中时按后缀模糊匹配
func resolveFuzzy(input, rel string, roots []string) (Match, error) {
	candidates := fuzzyBySuffix(rel, roots)
	switch len(candidates) {
	case 0:
		return Match{}, fmt.Errorf("未找到文件：%s", input)
	case 1:
		return matchOf(candidates[0]), nil
	default:
		return Match{}, &AmbiguousError{Input: input, Candidates: candidates}
	}
}

// exactMatches 精确候选：相对 LocalDir 的完整路径 + 各搜索根下的相对路径
func exactMatches(rel string, roots []string) []string {
	candidates := make([]string, 0, len(roots)+1)
	if fileExistsRel(rel) {
		candidates = append(candidates, rel)
	}
	for _, root := range roots {
		rooted := root + "/" + rel
		if fileExistsRel(rooted) {
			candidates = append(candidates, rooted)
		}
	}
	return uniqueSorted(candidates)
}

// fuzzyByBasename 按文件名（含去扩展名）匹配
func fuzzyByBasename(base string, roots []string) []string {
	candidates := make([]string, 0, 8)
	for _, root := range roots {
		index := indexFor(root)
		candidates = append(candidates, index.byBase[base]...)
		if !strings.Contains(base, ".") {
			candidates = append(candidates, index.byStem[base]...)
		}
	}
	return uniqueSorted(candidates)
}

// fuzzyBySuffix 按路径后缀匹配（notes/a.md 也能命中 documents/x/notes/a.md）
func fuzzyBySuffix(rel string, roots []string) []string {
	lower := strings.ToLower(rel)
	base := filepath.Base(filepath.FromSlash(lower))
	candidates := make([]string, 0, 8)
	for _, root := range roots {
		index := indexFor(root)
		for _, candidate := range index.byBase[base] {
			lowerCandidate := strings.ToLower(candidate)
			if lowerCandidate == lower || strings.HasSuffix(lowerCandidate, "/"+lower) {
				candidates = append(candidates, candidate)
			}
		}
	}
	return uniqueSorted(candidates)
}

// matchOf 由相对路径构造定位结果（存在性由调用方校验）
func matchOf(rel string) Match {
	rel = filepath.ToSlash(strings.Trim(rel, "/"))
	return Match{
		Abs: filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel)),
		Rel: rel,
	}
}

// fileExistsRel 判断相对 LocalDir 的路径是否为存在的文件
func fileExistsRel(rel string) bool {
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}

// normalizeInput 统一分隔符并去首尾空白
func normalizeInput(input string) string {
	return strings.TrimSpace(strings.ReplaceAll(input, `\`, "/"))
}

// uniqueSorted 去重并排序，保证候选顺序稳定
func uniqueSorted(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
