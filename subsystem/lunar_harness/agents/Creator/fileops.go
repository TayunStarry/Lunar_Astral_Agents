package Creator

// fileops.go /document 根目录内的文件操作。
// 所有路径都相对 /document（物理目录为 LocalDir/documents），
// 绝对路径、.. 穿越与非法字符一律拒绝，写入后登记统一资源句柄。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// documentsAbsRoot 返回 /document 的绝对路径。
func documentsAbsRoot() string {
	abs, err := filepath.Abs(filepath.Join(config.LocalDir, documentsRootRel))
	if err != nil {
		return filepath.Join(config.LocalDir, documentsRootRel)
	}
	return abs
}

// documentPath 校验并解析相对 /document 的路径。
// 返回绝对路径与相对 LocalDir 的路径（用于句柄登记）。
func documentPath(rel string) (full string, localRel string, err error) {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	switch {
	case rel == "/document":
		rel = ""
	case strings.HasPrefix(rel, "/document/"):
		rel = strings.TrimPrefix(rel, "/document/")
	case strings.HasPrefix(rel, "/"):
		return "", "", fmt.Errorf("请使用相对 /document 的路径")
	}
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

// listDocuments 列出目录内容；path 为空表示根目录。
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
	base := strings.TrimPrefix(strings.TrimPrefix(localRel, documentsRootRel+"/"), documentsRootRel)
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

// readDocument 读取文本文件，超长按上限截断。
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

// createDocument 创建文件；mustCreate 为 true 时目标已存在则报错。
func createDocument(rel, content string, mustCreate bool) (string, error) {
	full, localRel, err := documentPath(rel)
	if err != nil {
		return "", err
	}
	if mustCreate {
		if _, statErr := os.Stat(full); statErr == nil {
			return "", fmt.Errorf("文件已存在，请改用 file_rewrite")
		}
	}
	if err := persistence.WriteFileAtomic(localRel, []byte(content)); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	registerDocumentHandle(localRel)
	return localRel, nil
}

// removeDocumentContent 清空文件内容但保留文件。
func removeDocumentContent(rel string) error {
	_, localRel, err := documentPath(rel)
	if err != nil {
		return err
	}
	if err := persistence.WriteFileAtomic(localRel, nil); err != nil {
		return fmt.Errorf("清空文件失败: %w", err)
	}
	registerDocumentHandle(localRel)
	return nil
}

// deleteDocument 删除文件并清理句柄。
func deleteDocument(rel string) error {
	full, localRel, err := documentPath(rel)
	if err != nil {
		return err
	}
	info, statErr := os.Stat(full)
	if statErr != nil {
		return fmt.Errorf("文件不存在: %w", statErr)
	}
	if info.IsDir() {
		return fmt.Errorf("目标是一个目录，拒绝删除")
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("删除文件失败: %w", err)
	}
	resource.RemoveHandlesForPath(localRel)
	return nil
}

// moveDocument 移动或重命名文件，并迁移句柄。
func moveDocument(from, target string) error {
	fullFrom, localFrom, err := documentPath(from)
	if err != nil {
		return err
	}
	fullTarget, localTarget, err := documentPath(target)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(fullFrom); statErr != nil {
		return fmt.Errorf("源文件不存在: %w", statErr)
	}
	if _, statErr := os.Stat(fullTarget); statErr == nil {
		return fmt.Errorf("目标路径已存在")
	}
	if err := os.MkdirAll(filepath.Dir(fullTarget), 0o755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}
	if err := os.Rename(fullFrom, fullTarget); err != nil {
		return fmt.Errorf("移动文件失败: %w", err)
	}
	resource.RewriteHandlesForMove(localFrom, localTarget)
	return nil
}

// replaceDocument 替换文件内容，返回替换次数。
func replaceDocument(rel, search, replace string, all bool) (int, error) {
	if search == "" {
		return 0, fmt.Errorf("要查找的原文不能为空")
	}
	text, err := readDocument(rel)
	if err != nil {
		return 0, err
	}
	count := strings.Count(text, search)
	if count == 0 {
		return 0, fmt.Errorf("未找到要替换的内容")
	}
	next := text
	if all {
		next = strings.ReplaceAll(text, search, replace)
	} else {
		next = strings.Replace(text, search, replace, 1)
		count = 1
	}
	if _, err := createDocument(rel, next, false); err != nil {
		return 0, err
	}
	return count, nil
}

// deleteDocumentText 删除文件中指定原文，返回删除次数。
func deleteDocumentText(rel, text string, all bool) (int, error) {
	if text == "" {
		return 0, fmt.Errorf("要删除的原文不能为空")
	}
	content, err := readDocument(rel)
	if err != nil {
		return 0, err
	}
	count := strings.Count(content, text)
	if count == 0 {
		return 0, fmt.Errorf("未找到要删除的内容")
	}
	next := content
	if all {
		next = strings.ReplaceAll(content, text, "")
	} else {
		next = strings.Replace(content, text, "", 1)
		count = 1
	}
	if _, err := createDocument(rel, next, false); err != nil {
		return 0, err
	}
	return count, nil
}

// registerDocumentHandle 写入后登记统一资源句柄，返回句柄文本（失败时为空）。
func registerDocumentHandle(localRel string) string {
	_, handle, err := resource.RegisterPath(localRel)
	if err != nil {
		return ""
	}
	return handle
}
