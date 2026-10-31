package resource

// read.go 按句柄取回内容
//
// 语义：
//   - 本地实体在位 → 文本按锚点展开，二进制返回 base64；
//   - 实体丢失 → Lost=true；有描述保留句柄，无描述进入 GC 表 1；
//   - 外部链接（无本地实体）→ 返回「地址 + 标题 + 解析文本」，不触发加载。

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
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
	// GC 表 2：月华通过文件查询调用该句柄时，实时把「当前调用」置 1
	archiveDB.markGCCurrentCall(res.Handle)

	result := ReadResult{
		Handle:   res.Handle,
		Kind:     res.Kind,
		Title:    displayTitle(res),
		MIME:     res.MIME,
		BlobPath: res.BlobPath,
	}

	// 无本地实体：外部链接按「地址 + 标题 + 解析文本」取回；
	// 悬空句柄（如音频转写）直接返回其描述文本，不标记丢失。
	if res.BlobPath == "" {
		if res.Kind == KindURL || IsRemoteURL(res.URL) {
			result.Content = formatURLContent(res)
			result.TotalLines = strings.Count(result.Content, "\n") + 1
			return result, nil
		}
		result.Content = strings.TrimSpace(res.ParseText)
		result.Lost = res.State == StateLost || result.Content == ""
		return result, nil
	}

	abs := localAbs(res.BlobPath)
	if res.State != StateActive || !fileExists(abs) {
		// 有文本描述：句柄保留并降级为 lost；无描述：移出主表并进入 GC 表 1
		_ = archiveDB.preserveOrArchive(res, gcOrphanReason(res))
		result.Lost = true
		result.Content = strings.TrimSpace(res.ParseText)
		return result, nil
	}

	if maxBytes <= 0 {
		maxBytes = MaxReadBytes
	}
	if IsTextKind(res.Kind) && !isBinaryResource(res, abs) {
		return readTextResource(res, result, handle, abs, maxBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		_ = archiveDB.preserveOrArchive(res, gcOrphanReason(res))
		result.Lost = true
		result.Content = strings.TrimSpace(res.ParseText)
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
		_ = archiveDB.preserveOrArchive(res, gcOrphanReason(res))
		base.Lost = true
		base.Content = strings.TrimSpace(res.ParseText)
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

// isBinaryResource 判断 @D 资源应按文本还是二进制取回。
// 先按扩展名（文本白名单 / 二进制黑名单），未知扩展名再看文件头：
// 出现 NUL 或控制字符占比过高即判定为不可读文本，按 base64 取回。
func isBinaryResource(res Resource, abs string) bool {
	ext := strings.ToLower(res.Ext)
	if ext == "" {
		ext = strings.ToLower(localExt(res.BlobPath))
	}
	if workspaceTextExtensions[ext] {
		return false
	}
	if binaryExtensions[ext] {
		return true
	}
	return sniffBinary(abs)
}

// localExt 从相对路径取扩展名（resource.Ext 为空时的兜底）。
func localExt(rel string) string {
	if idx := strings.LastIndex(rel, "."); idx >= 0 {
		return rel[idx:]
	}
	return ""
}

// sniffBinary 读取文件头 512 字节做轻量二进制探测。
func sniffBinary(abs string) bool {
	file, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := file.Read(head)
	if n == 0 {
		return false
	}
	head = head[:n]
	controls := 0
	for _, b := range head {
		if b == 0 {
			return true
		}
		if b < 0x09 || (b > 0x0D && b < 0x20) {
			controls++
		}
	}
	return controls*100/n > 8
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
