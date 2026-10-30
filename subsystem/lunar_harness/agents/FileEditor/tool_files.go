package FileEditor

// tool_files.go 文件类工具实现：读取 / 写入 / 创建 / 删除 / 复制 / 移动 / 重命名 /
// 内容检索 / 指定位置置换 / 属性查看。全部返回结构化 JSON（含 handles 与 summary）。

import (
	"fmt"
	"os"
	"strings"
)

// FileList 列出文档库目录。
func FileList(path string) OpResult {
	entries, err := listDocuments(path)
	if err != nil {
		return failResult(ToolList, path, err)
	}
	return newResult(ToolList, path, fmt.Sprintf("列出 %d 个条目", len(entries)), nil,
		map[string]any{"entries": entries})
}

// ReadFile 读取文件内容（可分页），并上报「已读取」。
func ReadFile(path string, offset, maxChars int) OpResult {
	content, err := readDocument(path)
	if err != nil {
		return failResult(ToolRead, path, err)
	}
	runes := []rune(content)
	total := len(runes)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	size := maxChars
	if size <= 0 || size > fileReadChunkLimit {
		size = fileReadChunkRunes
	}
	end := offset + size
	if end > total {
		end = total
	}
	_, localRel, pathErr := documentPath(path)
	var handles []string
	if pathErr == nil {
		if handle := registerHandle(localRel); handle != "" {
			handles = append(handles, handle)
		}
		publish(localRel, "read", "", 0, 0)
	}
	detail := map[string]any{
		"offset":      offset,
		"max_chars":   size,
		"total_chars": total,
		"has_more":    end < total,
		"content":     string(runes[offset:end]),
	}
	summary := fmt.Sprintf("读取文件 %s（%d-%d / %d 字）", path, offset, end, total)
	if end < total {
		detail["next_offset"] = end
		summary += fmt.Sprintf("，可继续用 offset=%d 读取", end)
	}
	return newResult(ToolRead, path, summary, handles, detail)
}

// WriteFile 写入文件（不存在则创建）。
func WriteFile(path, content string) OpResult {
	localRel, handle, err := writeDocument(path, content, false)
	if err != nil {
		return failResult(ToolWrite, path, err)
	}
	publish(localRel, "write", content, 1, lineCount(content))
	handles := []string{}
	if handle != "" {
		handles = append(handles, handle)
	}
	return newResult(ToolWrite, path,
		fmt.Sprintf("写入文件 %s（%d 行 / %d 字节）", path, lineCount(content), len([]byte(content))),
		handles, map[string]any{"lines": lineCount(content), "bytes": len([]byte(content))})
}

// CreateFile 新建文件（已存在则报错）。
func CreateFile(path, content string) OpResult {
	localRel, handle, err := writeDocument(path, content, true)
	if err != nil {
		return failResult(ToolCreate, path, err)
	}
	publish(localRel, "create", content, 1, lineCount(content))
	handles := []string{}
	if handle != "" {
		handles = append(handles, handle)
	}
	return newResult(ToolCreate, path,
		fmt.Sprintf("创建文件 %s（%d 行）", path, lineCount(content)),
		handles, map[string]any{"lines": lineCount(content)})
}

// DeleteFile 删除文件。
func DeleteFile(path string) OpResult {
	localRel, err := deleteDocument(path)
	if err != nil {
		return failResult(ToolDelete, path, err)
	}
	publish(localRel, "delete", "", 0, 0)
	return newResult(ToolDelete, path, "删除文件 "+path, nil, nil)
}

// CopyFile 复制文件。
func CopyFile(path, target string) OpResult {
	localRel, handle, err := copyDocument(path, target)
	if err != nil {
		return failResult(ToolCopy, path, err)
	}
	publish(localRel, "copy", "", 0, 0)
	handles := []string{}
	if handle != "" {
		handles = append(handles, handle)
	}
	return newResult(ToolCopy, target, fmt.Sprintf("复制 %s → %s", path, target), handles, nil)
}

// MoveFile 移动文件。
func MoveFile(path, target string) OpResult {
	localRel, err := moveDocument(path, target)
	if err != nil {
		return failResult(ToolMove, path, err)
	}
	publish(localRel, "move", "", 0, 0)
	handles := []string{}
	if handle := registerHandle(localRel); handle != "" {
		handles = append(handles, handle)
	}
	return newResult(ToolMove, target, fmt.Sprintf("移动 %s → %s", path, target), handles, nil)
}

// RenameFile 重命名（同目录内改名）。
func RenameFile(path, name string) OpResult {
	name = strings.TrimSpace(name)
	if name == "" {
		return failResult(ToolRename, path, fmt.Errorf("新文件名不能为空"))
	}
	if strings.ContainsAny(name, "/\\") {
		return failResult(ToolRename, path, fmt.Errorf("新文件名不能包含路径分隔符"))
	}
	dir := path
	if idx := strings.LastIndex(strings.ReplaceAll(path, "\\", "/"), "/"); idx >= 0 {
		dir = path[:idx+1]
	} else {
		dir = ""
	}
	target := dir + name
	localRel, err := moveDocument(path, target)
	if err != nil {
		return failResult(ToolRename, path, err)
	}
	publish(localRel, "rename", "", 0, 0)
	handles := []string{}
	if handle := registerHandle(localRel); handle != "" {
		handles = append(handles, handle)
	}
	return newResult(ToolRename, target, fmt.Sprintf("重命名 %s → %s", path, target), handles, nil)
}

// SearchContent 检索文件或目录内容。
func SearchContent(path, query string) OpResult {
	hits, err := searchDocument(path, query)
	if err != nil {
		return failResult(ToolSearch, path, err)
	}
	// 单文件检索同样算「本次运行读取过的文件」；目录检索不登记具体文件。
	if full, err := AbsPath(path); err == nil {
		if info, statErr := os.Stat(full); statErr == nil && !info.IsDir() {
			if _, localRel, pathErr := documentPath(path); pathErr == nil {
				publish(localRel, "search", "", 0, 0)
			}
		}
	}
	if len(hits) == 0 {
		return newResult(ToolSearch, path,
			fmt.Sprintf("在 %s 中未找到「%s」", emptyAsDir(path), query), nil,
			map[string]any{"hits": []searchHit{}, "count": 0})
	}
	return newResult(ToolSearch, path,
		fmt.Sprintf("检索「%s」命中 %d 处", query, len(hits)), nil,
		map[string]any{"hits": hits, "count": len(hits)})
}

// ReplaceAt 置换指定行区间的内容。
func ReplaceAt(path string, start, end int, content string) OpResult {
	localRel, newStart, snippetLines, err := replaceRange(path, start, end, content)
	if err != nil {
		return failResult(ToolReplace, path, err)
	}
	publish(localRel, "replace", content, newStart, snippetLines)
	handles := []string{}
	if handle := registerHandle(localRel); handle != "" {
		handles = append(handles, handle)
	}
	endLabel := fmt.Sprintf("%d", end)
	if end < start {
		endLabel = fmt.Sprintf("%d", start)
	}
	return newResult(ToolReplace, path,
		fmt.Sprintf("置换 %s 第 %d-%s 行（写入 %d 行）", path, start, endLabel, snippetLines),
		handles, map[string]any{"start_line": newStart, "lines": snippetLines})
}

// FileAttr 查看文件属性。
func FileAttr(path string) OpResult {
	detail, err := fileInfo(path)
	if err != nil {
		return failResult(ToolInfo, path, err)
	}
	handles := []string{}
	if handle, ok := detail["handle"].(string); ok && handle != "" {
		handles = append(handles, handle)
	}
	_, localRel, pathErr := documentPath(path)
	if pathErr == nil {
		publish(localRel, "info", "", 0, 0)
	}
	return newResult(ToolInfo, path,
		fmt.Sprintf("%s：%d 字节 / %v 行", path, toInt64(detail["size"]), detail["lines"]),
		handles, detail)
}

// ==== 小工具 ====

// lineCount 文本行数（末尾换行不额外计一行）。
func lineCount(text string) int {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func emptyAsDir(path string) string {
	if strings.TrimSpace(path) == "" {
		return "文档库"
	}
	return path
}

func toInt64(v any) int64 {
	switch value := v.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	}
	return 0
}

// previewRunes 按字符截断。
func previewRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
