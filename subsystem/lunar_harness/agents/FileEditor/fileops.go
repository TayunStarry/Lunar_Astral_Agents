package FileEditor

// fileops.go /document 根目录内的文件操作与句柄登记。
// 所有路径都相对 /document（物理目录为 LocalDir/documents），
// 绝对路径、.. 穿越与非法字符一律拒绝；每次操作后向资源库状态中心上报。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/library"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// documentsAbsRoot 返回 /document 的绝对路径。
func documentsAbsRoot() string {
	abs, err := filepath.Abs(filepath.Join(config.LocalDir, documentsRootRel))
	if err != nil {
		return filepath.Join(config.LocalDir, documentsRootRel)
	}
	return abs
}

// documentPath 解析目标文件：句柄、完整路径、相对路径或纯文件名。
//
// 精确路径存在时直接使用；不存在时交给统一文件定位器做模糊匹配（纯文件名 / 路径后缀），
// 同名的多个候选会返回带候选列表的歧义错误，供调用方选择。
func documentPath(rel string) (full string, localRel string, err error) {
	raw := strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	// 句柄与空路径走严格解析
	if strings.HasPrefix(raw, "@") || raw == "" || raw == "/document" {
		return strictDocumentPath(raw)
	}
	full, localRel, err = strictDocumentPath(raw)
	if err == nil {
		if _, statErr := os.Stat(full); statErr == nil {
			return full, localRel, nil
		}
	}
	match, locateErr := resourcehub.Resolve(raw, []string{documentsRootRel})
	if locateErr == nil && strings.HasPrefix(match.Rel, documentsRootRel+"/") {
		return match.Abs, match.Rel, nil
	}
	if _, ambiguous := locateErr.(*resourcehub.AmbiguousError); ambiguous {
		return "", "", locateErr
	}
	if err != nil {
		return "", "", err
	}
	// 未命中已有文件：保留严格路径，供创建类操作使用
	return full, localRel, nil
}

// strictDocumentPath 只做路径文法校验与拼接，不做模糊匹配。
func strictDocumentPath(rel string) (full string, localRel string, err error) {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	if strings.HasPrefix(rel, "@") {
		resolved, resolveErr := handleDocumentPath(rel)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		rel = resolved
	}
	switch {
	case rel == "/document":
		rel = ""
	case strings.HasPrefix(rel, "/document/"):
		rel = strings.TrimPrefix(rel, "/document/")
	case strings.HasPrefix(rel, "/"):
		return "", "", fmt.Errorf("请使用相对 /document 的路径")
	}
	// 兼容调用方把文档库前缀写进路径（否则会落到 documents/documents/ 下）
	rel = strings.TrimPrefix(rel, documentsRootRel+"/")
	rel = strings.TrimPrefix(rel, "document/")
	if strings.ContainsRune(rel, 0) {
		return "", "", fmt.Errorf("路径包含非法字符")
	}
	if strings.Contains(rel, "..") {
		return "", "", fmt.Errorf("路径不允许包含 ..")
	}
	if filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("请使用相对 /document 的路径")
	}
	root := documentsAbsRoot()
	cleaned := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if cleaned != root && !strings.HasPrefix(cleaned, root+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("路径越出 /document 根目录")
	}
	local := filepath.ToSlash(filepath.Join(documentsRootRel, rel))
	return cleaned, local, nil
}

// handleDocumentPath 把 @D.xxx 句柄解析成相对 /document 的路径（忽略章节 / 行锚点）
func handleDocumentPath(raw string) (string, error) {
	parsed, ok := resource.ParseHandle(raw)
	if !ok || parsed.Kind != resource.KindDocument {
		return "", fmt.Errorf("只支持文档句柄（@D.xxx）：%s", raw)
	}
	handle := resource.FormatHandle(parsed.Kind, parsed.Seq)
	entry, found := resource.Lookup(handle)
	if !found {
		return "", fmt.Errorf("句柄不存在：%s", handle)
	}
	blob := filepath.ToSlash(entry.BlobPath)
	if !strings.HasPrefix(blob, documentsRootRel+"/") {
		return "", fmt.Errorf("句柄 %s 不在 /document 文档库内", handle)
	}
	return strings.TrimPrefix(blob, documentsRootRel+"/"), nil
}

// docRelative 把相对 LocalDir 的路径转回相对 documents（用于控制台上报）。
func docRelative(localRel string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(localRel), documentsRootRel+"/")
	return normalizeDocPath(rel)
}

// normalizeDocPath 规整相对 documents 的路径。
func normalizeDocPath(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	path = strings.TrimPrefix(path, "/document/")
	path = strings.TrimPrefix(path, "document/")
	path = strings.TrimPrefix(path, "documents/")
	path = strings.TrimPrefix(path, "/")
	return path
}

// publish 向资源库上报一次操作（读取类操作不提供改动片段）。
func publish(localRel, op, added, removed string, startLine, addedLines int) {
	handle := registerHandle(localRel)
	library.PublishEdit(library.EditNotice{
		Path:      filepath.ToSlash(localRel),
		Op:        op,
		Added:     added,
		Removed:   removed,
		StartLine: startLine,
		Lines:     addedLines,
		Handle:    handle,
		At:        time.Now(),
	})
}

// publishRun 上报一次「运行项目文件」的结果（failed=true 时文件名标红）。
func publishRun(localRel string, failed bool) {
	library.PublishEdit(library.EditNotice{
		Path:   filepath.ToSlash(localRel),
		Op:     "run",
		Handle: registerHandle(localRel),
		Failed: failed,
		At:     time.Now(),
	})
}

// existingText 读取操作前的内容（不存在、目录或超过读取上限时返回空串）。
func existingText(rel string) string {
	text, err := readDocument(rel)
	if err != nil {
		return ""
	}
	return text
}

// AbsPath 把相对 /document 的路径解析为绝对路径（供同进程内的其它子系统复用路径校验）。
func AbsPath(rel string) (string, error) {
	full, _, err := documentPath(rel)
	return full, err
}

// ReadText 读取文件全文（供同进程内的其它子系统复用大小与目录校验）。
func ReadText(rel string) (string, error) { return readDocument(rel) }

// ==== 读取与列出 ====

// listDocuments 列出目录内容；rel 为空表示根目录。
func listDocuments(rel string) ([]fileEntry, error) {
	full, localRel, err := documentPath(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}
	out := make([]fileEntry, 0, len(entries))
	base := docRelative(localRel)
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		name := entry.Name()
		path := name
		if base != "" {
			path = filepath.ToSlash(filepath.Join(base, name))
		}
		out = append(out, fileEntry{
			Name:  name,
			Path:  path,
			Dir:   entry.IsDir(),
			Size:  info.Size(),
			MTime: info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// readDocument 读取文本文件，超长时报错（大文件用 offset 分批读）。
func readDocument(rel string) (string, error) {
	full, _, err := documentPath(rel)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("文件不存在: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("目标是一个目录，请用 file_list")
	}
	if info.Size() > maxFileReadBytes {
		return "", fmt.Errorf("文件过大（%d 字节），上限 %d 字节", info.Size(), maxFileReadBytes)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	return string(raw), nil
}

// ==== 写入类操作 ====

// writeDocument 写入文件（upsert）；mustCreate 为 true 时目标已存在则报错。
func writeDocument(rel, content string, mustCreate bool) (string, string, error) {
	full, localRel, err := documentPath(rel)
	if err != nil {
		return "", "", err
	}
	if mustCreate {
		if _, statErr := os.Stat(full); statErr == nil {
			return "", "", fmt.Errorf("文件已存在，请改用 file_write")
		}
	}
	if err := persistence.WriteFileAtomic(localRel, []byte(content)); err != nil {
		return "", "", fmt.Errorf("写入文件失败: %w", err)
	}
	resourcehub.Invalidate()
	return localRel, registerHandle(localRel), nil
}

// deleteDocument 删除文件并清理句柄。
func deleteDocument(rel string) (string, error) {
	full, localRel, err := documentPath(rel)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(full)
	if statErr != nil {
		return "", fmt.Errorf("文件不存在: %w", statErr)
	}
	if info.IsDir() {
		return "", fmt.Errorf("目标是一个目录，拒绝删除")
	}
	if err := os.Remove(full); err != nil {
		return "", fmt.Errorf("删除文件失败: %w", err)
	}
	resourcehub.Invalidate()
	resource.RemoveHandlesForPath(localRel)
	return localRel, nil
}

// copyDocument 复制文件（目标必须不存在）。
func copyDocument(from, target string) (string, string, error) {
	content, err := readDocument(from)
	if err != nil {
		return "", "", err
	}
	return writeDocument(target, content, true)
}

// moveDocument 移动或重命名文件，并迁移句柄。
func moveDocument(from, target string) (string, error) {
	fullFrom, localFrom, err := documentPath(from)
	if err != nil {
		return "", err
	}
	fullTarget, localTarget, err := documentPath(target)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(fullFrom); statErr != nil {
		return "", fmt.Errorf("源文件不存在: %w", statErr)
	}
	if _, statErr := os.Stat(fullTarget); statErr == nil {
		return "", fmt.Errorf("目标路径已存在")
	}
	if err := os.MkdirAll(filepath.Dir(fullTarget), 0o755); err != nil {
		return "", fmt.Errorf("创建目标目录失败: %w", err)
	}
	if err := os.Rename(fullFrom, fullTarget); err != nil {
		return "", fmt.Errorf("移动文件失败: %w", err)
	}
	resourcehub.Invalidate()
	resource.RewriteHandlesForMove(localFrom, localTarget)
	return localTarget, nil
}

// ==== 内容检索与置换 ====

// searchDocument 在文件或目录中检索内容（目录递归扫描文本文件）。
func searchDocument(rel, query string) ([]searchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("检索内容不能为空")
	}
	full, _, err := documentPath(rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %w", err)
	}
	if !info.IsDir() {
		return searchInFile(rel, query, full)
	}
	root := full
	hits := make([]searchHit, 0, 8)
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(hits) >= searchHitLimit {
			return nil
		}
		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		docRel := filepath.ToSlash(filepath.Join(normalizeDocPath(rel), relPath))
		fileHits, _ := searchInFile(docRel, query, path)
		hits = append(hits, fileHits...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	// 文件名命中也要返回，便于按名字找文件
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(hits) >= searchHitLimit {
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), strings.ToLower(query)) {
			relPath, _ := filepath.Rel(root, path)
			hits = append(hits, searchHit{
				Path: filepath.ToSlash(filepath.Join(normalizeDocPath(rel), relPath)),
				Text: "文件名命中：" + d.Name(),
			})
		}
		return nil
	})
	return hits, nil
}

// searchInFile 单个文件的内容检索（按行返回，最多 searchHitLimit 条）。
func searchInFile(docRel, query, full string) ([]searchHit, error) {
	info, err := os.Stat(full)
	if err != nil || info.Size() > searchScanBytes {
		return nil, nil
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, nil
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	needle := strings.ToLower(query)
	hits := make([]searchHit, 0, 4)
	for index, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(line), needle) {
			hits = append(hits, searchHit{
				Path: docRel,
				Line: index + 1,
				Text: previewRunes(strings.TrimSpace(line), 512),
			})
			if len(hits) >= searchHitLimit {
				break
			}
		}
	}
	return hits, nil
}

// replaceRange 用 content 替换文件第 [start,end] 行（1 基、含端点；内容行数可任意增减）。
// 返回新的起始行、写入片段行数与被替换掉的原文，供资源库差异渲染。
func replaceRange(rel string, start, end int, content string) (string, int, int, string, error) {
	if start < 1 {
		return "", 0, 0, "", fmt.Errorf("起始行必须 ≥ 1")
	}
	if end < start {
		end = start
	}
	text, err := readDocument(rel)
	if err != nil {
		return "", 0, 0, "", err
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if start > len(lines)+1 {
		return "", 0, 0, "", fmt.Errorf("起始行 %d 超出文件总行数 %d", start, len(lines))
	}
	if end > len(lines) {
		end = len(lines)
	}
	removed := ""
	if start <= len(lines) {
		removed = strings.Join(lines[start-1:end], "\n")
	}
	replacement := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	next := make([]string, 0, len(lines)+len(replacement))
	next = append(next, lines[:start-1]...)
	next = append(next, replacement...)
	next = append(next, lines[end:]...)
	localRel, _, err := writeDocument(rel, strings.Join(next, "\n"), false)
	if err != nil {
		return "", 0, 0, "", err
	}
	return localRel, start, len(replacement), removed, nil
}

// registerHandle 登记资源句柄（失败时返回空串）。
func registerHandle(localRel string) string {
	_, handle, err := resource.RegisterPath(localRel)
	if err != nil {
		return ""
	}
	return handle
}

// fileInfo 文件属性。
func fileInfo(rel string) (map[string]any, error) {
	full, localRel, err := documentPath(rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("文件不存在: %w", err)
	}
	detail := map[string]any{
		"name":     info.Name(),
		"dir":      info.IsDir(),
		"size":     info.Size(),
		"mode":     info.Mode().String(),
		"modified": info.ModTime().Format("2006-01-02 15:04:05"),
		"ext":      strings.ToLower(filepath.Ext(info.Name())),
	}
	if !info.IsDir() {
		if raw, readErr := os.ReadFile(full); readErr == nil {
			detail["lines"] = strings.Count(string(raw), "\n") + 1
			detail["chars"] = len([]rune(string(raw)))
		}
	}
	handle := registerHandle(localRel)
	if handle != "" {
		detail["handle"] = handle
	}
	return detail, nil
}
