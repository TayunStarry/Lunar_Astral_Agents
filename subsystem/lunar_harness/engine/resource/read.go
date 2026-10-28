package resource

// read.go 资源取回 / 列表 / 搜索 / 删除的对外接口 —— 句柄解析 + 整段展开
//
// 核心语义（ADR「本体与句柄解耦」）：
//   blob 在位 → 返回本体文本（文档按行区间切片；图片等二进制返回 base64）
//   blob 已丢失（GC 已改为连带删除目录行；此处兜住外部删除/损坏等意外丢失）
//     → 返回 Lost=true 并标记 lost，由上层按失效句柄清理并提示，不滞留
//
// 段落展开遵循「整段展开，绝不半句截断」：按 |§n 或 |L.. 定位到完整行区间。

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// Read 按句柄取回资源内容（带锚点时按锚点定位）
//
// rawHandle 形如 @D.003 / @D.003|§2 / @I.007 / @D.003|L120-168。
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
		Handle:   rawHandle,
		Kind:     res.Kind,
		Title:    displayTitle(res),
		MIME:     res.MIME,
		BlobPath: res.BlobPath,
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))

	// 链接资源不落地本体：取回即「地址 + 标题 + 缓存摘要」，不参与 blob 存在性检查
	if res.Kind == KindURL {
		if strings.TrimSpace(res.Summary) == "" {
			if summary, title := LookupURLCache(res.URL); summary != "" {
				_ = archiveDB.UpdateResourceURLExtras(res.ID, firstNonEmpty(title, res.Title), summary)
				res.Summary = summary
				if res.Title == "" {
					res.Title = title
				}
			}
		}
		result.Content = formatURLContent(res)
		result.TotalLines = strings.Count(result.Content, "\n") + 1
		return result, nil
	}

	// blob 丢失 / 不可读：移除失效句柄，按统一语义提示调用方
	if res.State != StateActive || !fileExists(abs) {
		_ = archiveDB.DeleteResource(res.ID)
		result.Lost = true
		return result, nil
	}

	if maxBytes <= 0 {
		maxBytes = MaxReadBytes
	}

	// 文本类资源（文档 / 表格）按文本取回
	if IsTextKind(res.Kind) && res.Kind != KindURL {
		return readTextResource(res, result, handle, abs, maxBytes)
	}

	// 二进制资源：返回 base64（图片交给多模态管线，视频/音频交给对应 B 分支）
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

// readTextResource 文本资源的取回：无锚点返回全文（可截断），带锚点返回整段行区间
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
		section, found := findSection(res.ID, handle.SectionOrdinal)
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

// findSection 按章节序号读取段落表条目
func findSection(resourceID int64, ordinal int) (Section, bool) {
	sections, err := archiveDB.ListSections(resourceID)
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
