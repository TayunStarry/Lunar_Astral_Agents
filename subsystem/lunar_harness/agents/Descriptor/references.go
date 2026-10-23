package Descriptor

// references.go 文件引用关系：扫描 import / require / include / Markdown 链接等
//
// 只做「本文件指向哪些文件」的正向解析，不扫描整个工作区做反向索引：
// 反向索引成本与误报都远高于此处的实际需求。

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// buildReferences 扫描全文的引用关系
func buildReferences(doc *docContext) []fileReference {
	ext := strings.ToLower(doc.Resource.Ext)
	refs := make([]fileReference, 0, 24)
	seen := map[string]bool{}
	inGoImport := false

	for index, raw := range doc.Lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		targets := make([]string, 0, 2)

		switch ext {
		case ".go":
			if strings.HasPrefix(trimmed, "import (") {
				inGoImport = true
				continue
			}
			if inGoImport && trimmed == ")" {
				inGoImport = false
				continue
			}
			if inGoImport {
				if match := reQuoted.FindStringSubmatch(trimmed); match != nil {
					targets = append(targets, match[1])
				}
			} else if strings.HasPrefix(trimmed, "import ") {
				if match := reQuoted.FindStringSubmatch(trimmed); match != nil {
					targets = append(targets, match[1])
				}
			}
		case ".md", ".markdown":
			targets = append(targets, matchesOf(reMDLink, line)...)
		default:
			targets = append(targets, matchesOf(reJSImport, line)...)
			targets = append(targets, matchesOf(reRequire, line)...)
			targets = append(targets, matchesOf(reInclude, line)...)
			targets = append(targets, matchesOf(reCSSImport, line)...)
			targets = append(targets, matchesOf(rePyFrom, line)...)
			targets = append(targets, matchesOf(rePyImport, line)...)
			switch ext {
			case ".html", ".htm", ".vue", ".tsx", ".jsx":
				targets = append(targets, matchesOf(reHTMLLink, line)...)
			}
		}

		for _, target := range targets {
			target = strings.TrimSpace(target)
			if skipReference(target) || !isFileLikeReference(target) {
				continue
			}
			key := fmt.Sprintf("%d|%s", index+1, target)
			if seen[key] {
				continue
			}
			seen[key] = true
			resolved, exists := resolveReference(doc, target)
			refs = append(refs, fileReference{
				Line:     index + 1,
				Raw:      truncateRunes(trimmed, 160),
				Target:   target,
				Resolved: resolved,
				Exists:   exists,
			})
			if len(refs) >= maxReferenceItems {
				return refs
			}
		}
	}
	return refs
}

// matchesOf 返回正则在文本中的首个捕获组
func matchesOf(pattern *regexp.Regexp, text string) []string {
	matches := pattern.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		for i := 1; i < len(match); i++ {
			if strings.TrimSpace(match[i]) != "" {
				out = append(out, match[i])
				break
			}
		}
	}
	return out
}

// skipReference 过滤外部链接与锚点
func skipReference(target string) bool {
	if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "/") {
		return true
	}
	lower := strings.ToLower(target)
	for _, prefix := range []string{"http://", "https://", "data:", "mailto:", "javascript:", "tel:"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// isFileLikeReference 过滤包名 / 命名空间这类非文件引用
//
// 例：Go 的 "fmt"、JS 的 "react" 不是项目内文件；相对路径或带扩展名的目标才是。
func isFileLikeReference(target string) bool {
	if strings.HasPrefix(target, ".") {
		return true
	}
	if strings.ContainsAny(target, "/\\") {
		return true
	}
	switch strings.ToLower(filepath.Ext(target)) {
	case ".md", ".markdown", ".txt", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini",
		".go", ".ts", ".tsx", ".js", ".jsx", ".vue", ".css", ".html", ".htm",
		".py", ".rs", ".java", ".c", ".h", ".cc", ".cpp", ".hpp", ".cs", ".kt", ".swift",
		".sql", ".ps1", ".sh", ".bat", ".lua", ".php", ".rb",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".mp4", ".webm", ".wav", ".mp3", ".pdf":
		return true
	}
	return false
}

// resolveReference 解析引用目标的相对路径并判断是否存在
func resolveReference(doc *docContext, target string) (string, bool) {
	clean := strings.TrimSpace(target)
	if cut := strings.IndexAny(clean, "?#"); cut >= 0 {
		clean = clean[:cut]
	}
	if decoded, err := url.PathUnescape(clean); err == nil {
		clean = decoded
	}
	clean = filepath.ToSlash(clean)
	if clean == "" {
		return "", false
	}

	bases := make([]string, 0, 2)
	if dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(doc.Resource.Name))); dir != "." && dir != "" {
		bases = append(bases, dir)
	}
	bases = append(bases, "")

	for _, base := range bases {
		rel := filepath.ToSlash(filepath.Clean(filepath.Join(base, clean)))
		if rel == "." {
			continue
		}
		local := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
		if fileExists(local) {
			return rel, true
		}
		if cwd, err := os.Getwd(); err == nil {
			if fileExists(filepath.Join(cwd, filepath.FromSlash(rel))) {
				return rel, true
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(filepath.Join(bases[0], clean))), false
}

// fileExists 判断路径是否为存在的普通文件
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// formatReferences 渲染引用关系清单（limit<=0 表示全部）
func formatReferences(refs []fileReference, limit int) string {
	if len(refs) == 0 {
		return "（未识别到文件引用）"
	}
	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	var builder strings.Builder
	for _, ref := range refs {
		state := "未找到"
		if ref.Exists {
			state = "存在"
		}
		fmt.Fprintf(&builder, "L%d %s → %s（%s）\n", ref.Line, ref.Target, ref.Resolved, state)
	}
	return strings.TrimRight(builder.String(), "\n")
}
