package resource

// read.go 按句柄取回内容
//
// 语义：
//   - 本地实体在位 → 文本按锚点展开，二进制返回 base64；
//   - 实体丢失 → Lost=true 并删除失效句柄；
//   - 外部链接（无本地实体）→ 返回「地址 + 标题 + 解析文本」，不触发加载。

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// Read 按句柄取回资源内容（带锚点时按锚点定位）
func Read(rawHandle string, maxBytes int) (ReadResult, error) {
	if err := EnsureReady(); err != nil {
		return ReadResult{}, err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok || handle.Seq <= 0 {
		return ReadResult{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := archiveDB.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return ReadResult{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	archiveDB.TouchResource(res.ID)

	result := ReadResult{
		Handle:   res.Handle,
		Kind:     res.Kind,
		Title:    displayTitle(res),
		MIME:     res.MIME,
		BlobPath: res.BlobPath,
	}

	// 外部链接：没有本地实体，按「地址 + 标题 + 解析文本」取回
	if res.BlobPath == "" {
		if res.Kind == KindURL || IsRemoteURL(res.URL) {
			result.Content = formatURLContent(res)
			result.TotalLines = strings.Count(result.Content, "\n") + 1
			return result, nil
		}
		result.Lost = true
		return result, nil
	}

	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
	if res.State != StateActive || !fileExists(abs) {
		_ = archiveDB.DeleteResource(res.ID)
		result.Lost = true
		return result, nil
	}

	if maxBytes <= 0 {
		maxBytes = MaxReadBytes
	}
	if IsTextKind(res.Kind) {
		return readTextResource(res, result, handle, abs, maxBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		_ = archiveDB.DeleteResource(res.ID)
		result.Lost = true
		return result, nil
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
		result.Truncated = true
	}
	result.Base64 = base64.StdEncoding.EncodeToString(data)
	return result, nil
}

// readTextResource 文本资源取回：无锚点整篇（可截断），带锚点整段行区间
func readTextResource(res Resource, base ReadResult, handle Handle, abs string, maxBytes int) (ReadResult, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		_ = archiveDB.DeleteResource(res.ID)
		base.Lost = true
		return base, nil
	}
	text := stripBOM(string(data))
	lines := strings.Split(text, "\n")
	base.TotalLines = len(lines)

	switch {
	case handle.HasLine:
		start, end := handle.LineStart, handle.LineEnd
		if start < 1 {
			start = 1
		}
		if end < start {
			end = start
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			start = len(lines)
		}
		base.Content = strings.Join(lines[start-1:end], "\n")
		base.LineStart, base.LineEnd = start, end
	case handle.HasSection:
		section, found := findSectionByURL(res.URL, handle.SectionOrdinal)
		if !found {
			return base, fmt.Errorf("章节不存在: %s（可用 outline 查看章节序号）", handle.Text())
		}
		base.SectionTitle = section.Title
		base.Content = joinLines(lines, section.LineStart, section.LineEnd)
		base.LineStart, base.LineEnd = section.LineStart, section.LineEnd
	default:
		base.Content = text
		base.LineStart, base.LineEnd = 1, len(lines)
	}

	if len(base.Content) > maxBytes {
		base.Content = base.Content[:maxBytes]
		base.Truncated = true
	}
	return base, nil
}

// findSectionByURL 按章节序号读取段落表条目
func findSectionByURL(rawURL string, ordinal int) (Section, bool) {
	sections, err := archiveDB.ListSectionsByURL(rawURL)
	if err != nil {
		return Section{}, false
	}
	for _, s := range sections {
		if s.Ordinal == ordinal {
			return s, true
		}
	}
	return Section{}, false
}

// joinLines 取 1 基行区间的文本（越界自动收敛）
func joinLines(lines []string, start, end int) string {
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end || start > len(lines) {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}
