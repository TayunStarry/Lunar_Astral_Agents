package DocumentReader

// folder.go 目录级读取：大型文件夹的读取统一交给文献解析智能体，
// 由本文件做确定性扫描（规模、结构、大文件、按需求摘录），不依赖模型。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// InspectFolderContext 读取一个目录并返回结构化摘要。
func InspectFolderContext(ctx context.Context, target, request string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	abs, rel, err := resolveFolderPath(target)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(abs)
	if statErr != nil {
		return "", fmt.Errorf("目录不存在: %w", statErr)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s 不是目录，请改用文件读取", target)
	}

	files := make([]folderFile, 0, maxFolderListFiles)
	dirs, totalBytes := 0, int64(0)
	scanned, truncated := 0, false
	walkErr := filepath.WalkDir(abs, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		scanned++
		if scanned > maxFolderScanEntries {
			truncated = true
			return filepath.SkipAll
		}
		if entry.IsDir() {
			if path != abs {
				dirs++
				if strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		fileRel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return nil
		}
		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		totalBytes += fileInfo.Size()
		files = append(files, scanFolderFile(path, filepath.ToSlash(fileRel), fileInfo.Size()))
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("扫描目录失败: %w", walkErr)
	}
	return renderFolderDigest(rel, files, dirs, totalBytes, truncated, request), nil
}

// scanFolderFile 读取单个文件的轻量预览信息。
func scanFolderFile(abs, rel string, size int64) folderFile {
	file := folderFile{Rel: rel, Size: size, Large: size > folderLargeBytes}
	if size > maxFolderPreviewBytes {
		file.Text = isTextName(rel)
		return file
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return file
	}
	head := raw
	if len(head) > profileHeadBytes {
		head = head[:profileHeadBytes]
	}
	if !isTextName(rel) || strings.IndexByte(string(head), 0) >= 0 {
		return file
	}
	file.Text = true
	text := strings.TrimPrefix(string(raw), "\ufeff")
	file.Lines = strings.Count(text, "\n") + 1
	file.Head = firstMeaningfulLines(strings.Split(text, "\n"), 1)
	return file
}

// isTextName 按扩展名判断是否可能为文本。
func isTextName(name string) bool {
	return !binaryExts[strings.ToLower(filepath.Ext(name))]
}

// renderFolderDigest 生成目录摘要。
func renderFolderDigest(rel string, files []folderFile, dirs int, totalBytes int64, truncated bool, request string) string {
	textFiles, binaryFiles := 0, 0
	for _, file := range files {
		if file.Text {
			textFiles++
		} else {
			binaryFiles++
		}
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "【目录】%s\n", rel)
	fmt.Fprintf(&builder, "规模：文件 %d（文本 %d / 二进制 %d）｜子目录 %d｜总计 %d 字节\n",
		len(files), textFiles, binaryFiles, dirs, totalBytes)
	if truncated {
		builder.WriteString("（扫描达到条目上限，仅统计前若干项）\n")
	}

	large := make([]folderFile, 0)
	for _, file := range files {
		if file.Large {
			large = append(large, file)
		}
	}
	if len(large) > 0 {
		sort.Slice(large, func(i, j int) bool { return large[i].Size > large[j].Size })
		builder.WriteString("\n【大文件】\n")
		for index, file := range large {
			if index >= 12 {
				fmt.Fprintf(&builder, "  （其余 %d 个未展开）\n", len(large)-index)
				break
			}
			fmt.Fprintf(&builder, "  %s（%d 字节）\n", file.Rel, file.Size)
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Size != files[j].Size {
			return files[i].Size > files[j].Size
		}
		return files[i].Rel < files[j].Rel
	})
	builder.WriteString("\n【文件清单】\n")
	limit := len(files)
	if limit > maxFolderListFiles {
		limit = maxFolderListFiles
	}
	for _, file := range files[:limit] {
		if !file.Text {
			fmt.Fprintf(&builder, "  %s（%d 字节，二进制）\n", file.Rel, file.Size)
			continue
		}
		line := strings.TrimSpace(file.Head)
		if line == "" {
			fmt.Fprintf(&builder, "  %s（%d 字节 / %d 行）\n", file.Rel, file.Size, file.Lines)
			continue
		}
		fmt.Fprintf(&builder, "  %s（%d 字节 / %d 行）｜%s\n", file.Rel, file.Size, file.Lines, agentbase.TruncateRunes(line, 512))
	}
	if limit < len(files) {
		fmt.Fprintf(&builder, "  （其余 %d 个文件未展开）\n", len(files)-limit)
	}

	if excerpt := folderExcerpt(rel, files, request); excerpt != "" {
		builder.WriteString("\n【按需求摘录】\n")
		builder.WriteString(excerpt)
		builder.WriteByte('\n')
	}
	return agentbase.HardCutRunes(strings.TrimRight(builder.String(), "\n"), maxFolderResultRunes)
}

// folderExcerpt 按需求关注词在文本文件中摘录命中行。
func folderExcerpt(rel string, files []folderFile, request string) string {
	terms := focusTerms(request)
	if len(terms) == 0 {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "关注点：%s\n", strings.Join(terms, "、"))
	excerptFiles := 0
	for _, file := range files {
		if excerptFiles >= maxFolderExcerptFiles {
			break
		}
		if !file.Text || file.Large {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel), filepath.FromSlash(file.Rel)))
		if err != nil {
			continue
		}
		raw, readErr := os.ReadFile(abs)
		if readErr != nil {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
		added := 0
		for index, line := range lines {
			if added >= maxFolderExcerptLines {
				break
			}
			lower := strings.ToLower(line)
			matched := ""
			for _, term := range terms {
				if strings.Contains(lower, strings.ToLower(term)) {
					matched = term
					break
				}
			}
			if matched == "" {
				continue
			}
			fmt.Fprintf(&builder, "%s L%d: %s\n", file.Rel, index+1, agentbase.TruncateRunes(strings.TrimSpace(line), 512))
			added++
		}
		if added > 0 {
			excerptFiles++
		}
	}
	if excerptFiles == 0 {
		return ""
	}
	return strings.TrimRight(builder.String(), "\n")
}

// focusTerms 从自然语言需求中提取关注词（去重、限长）。
func focusTerms(request string) []string {
	request = strings.TrimSpace(request)
	if request == "" {
		return nil
	}
	fields := strings.FieldsFunc(request, func(char rune) bool {
		if unicode.IsSpace(char) {
			return true
		}
		return strings.ContainsRune("，。；：、（）【】“”‘’!?,.", char)
	})
	terms := make([]string, 0, maxFolderTerms)
	seen := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len([]rune(field)) < 2 || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
		if len(terms) >= maxFolderTerms {
			break
		}
	}
	return terms
}

// resolveFolderPath 解析目录路径（/document、相对路径或本地绝对路径）。
func resolveFolderPath(target string) (abs, rel string, err error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", "", fmt.Errorf("目录路径不能为空")
	}
	if strings.HasPrefix(trimmed, "@") {
		return "", "", fmt.Errorf("目录读取请提供路径，不支持句柄: %s", target)
	}
	localRoot := filepath.Clean(config.LocalDir)
	switch {
	case trimmed == "/document" || strings.HasPrefix(trimmed, "/document/"):
		rel = "documents" + strings.TrimPrefix(trimmed, "/document")
	case filepath.IsAbs(filepath.FromSlash(trimmed)):
		abs = filepath.Clean(filepath.FromSlash(trimmed))
		relative, relErr := filepath.Rel(localRoot, abs)
		if relErr != nil {
			return "", "", fmt.Errorf("计算相对路径失败: %w", relErr)
		}
		rel = filepath.ToSlash(relative)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return "", "", fmt.Errorf("路径越出本地目录: %s", target)
		}
		return abs, rel, nil
	case strings.HasPrefix(trimmed, "/"):
		return "", "", fmt.Errorf("请使用 /document/ 前缀或相对路径: %s", target)
	default:
		cleaned := filepath.ToSlash(trimmed)
		cleaned = strings.TrimPrefix(cleaned, "./")
		if strings.HasPrefix(cleaned, "documents/") {
			rel = cleaned
		} else {
			docRel := "documents/" + cleaned
			docAbs := filepath.Join(localRoot, filepath.FromSlash(docRel))
			if info, statErr := os.Stat(docAbs); statErr == nil && info.IsDir() {
				rel = docRel
			} else {
				rel = cleaned
			}
		}
	}
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	abs = filepath.Join(localRoot, filepath.FromSlash(rel))
	if abs != localRoot && !strings.HasPrefix(abs, localRoot+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("路径越出本地目录: %s", target)
	}
	return abs, rel, nil
}
