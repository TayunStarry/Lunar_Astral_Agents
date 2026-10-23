package persistence

// archive_read.go 资源取回 —— 句柄解析 + 整段展开
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

	"LunarSubsystem/GeneralConfig"
)

// ReadResult 资源取回结果
type ReadResult struct {
	// Handle 规范句柄（含锚点，若请求带锚点）
	Handle string `json:"handle"`
	// Kind 类型码
	Kind string `json:"kind"`
	// Title 展示标题
	Title string `json:"title"`
	// Content 文本内容（文档 / 文本类资源；携带锚点时为其行区间）
	Content string `json:"content,omitempty"`
	// Base64 二进制内容（图片 / 视频 / 音频本体）
	//
	// 图片走思维链时**不是**原图：lunarloop 的资源取回适配器会把 KindImage 换成
	// 视觉读取副本（≤ vision_max_side）再填这里，原图字节不出 Go 侧（见 docs/adr/0008）
	Base64 string `json:"base64,omitempty"`
	// MIME 规范 MIME
	MIME string `json:"mime"`
	// Width / Height 图片副本的像素尺寸（仅图片取回时填写，供调用方取比例基准）
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// BlobPath 内容仓库路径（相对 LocalDir，可交给图片处理管线取回）
	BlobPath string `json:"blob_path"`
	// Lost blob 已丢失：句柄有效但本体不可取，调用方应降级为描述
	Lost bool `json:"lost"`
	// Truncated 内容按上限被截断
	Truncated bool `json:"truncated"`
	// SectionTitle 命中的章节标题（携带锚点时）
	SectionTitle string `json:"section_title,omitempty"`
	// LineStart / LineEnd 实际返回的行区间（1 基，含端点）
	LineStart int `json:"line_start,omitempty"`
	LineEnd   int `json:"line_end,omitempty"`
	// TotalLines 资源总行数（文本文档）
	TotalLines int `json:"total_lines,omitempty"`
}

// MaxReadBytes 单次取回内容的字节上限（防止一次读入超大文件撑爆上下文）
const MaxReadBytes = 256 * 1024

// ResourceRead 按句柄取回资源内容（带锚点时按锚点定位）
//
// rawHandle 形如 @D.003 / @D.003|§2 / @I.007 / @D.003|L120-168。
func ResourceRead(rawHandle string, maxBytes int) (ReadResult, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return ReadResult{}, err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok {
		return ReadResult{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := ArchiveDatabase.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return ReadResult{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	ArchiveDatabase.TouchResource(res.ID)

	result := ReadResult{
		Handle:   rawHandle,
		Kind:     res.Kind,
		Title:    displayTitle(res),
		MIME:     res.MIME,
		BlobPath: res.BlobPath,
	}
	abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath))

	// blob 丢失 / 不可读：移除失效句柄，按统一语义提示调用方
	if res.State != StateActive || !fileExists(abs) {
		_ = ArchiveDatabase.DeleteResource(res.ID)
		result.Lost = true
		return result, nil
	}

	if maxBytes <= 0 {
		maxBytes = MaxReadBytes
	}

	// 文本类资源（文档 / 表格）按文本取回
	if res.Kind == KindDocument || res.Kind == KindTable {
		return readTextResource(res, result, handle, abs, maxBytes)
	}

	// 二进制资源：返回 base64（图片交给多模态管线，视频/音频交给对应 B 分支）
	data, err := os.ReadFile(abs)
	if err != nil {
		_ = ArchiveDatabase.DeleteResource(res.ID)
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
func readTextResource(res ArchiveResource, base ReadResult, handle Handle, abs string, maxBytes int) (ReadResult, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		_ = ArchiveDatabase.DeleteResource(res.ID)
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
func findSection(resourceID int64, ordinal int) (ArchiveSection, bool) {
	sections, err := ArchiveDatabase.ListSections(resourceID)
	if err != nil {
		return ArchiveSection{}, false
	}
	for _, s := range sections {
		if s.Ordinal == ordinal {
			return s, true
		}
	}
	return ArchiveSection{}, false
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

// stripBOM 去除 UTF-8 BOM
func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// fileExists 判断路径是否为存在的普通文件
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ResourceOutline 返回资源的大纲（段落表 + 每节的规范锚点句柄）
func ResourceOutline(rawHandle string) ([]ArchiveSection, string, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return nil, "", err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok {
		return nil, "", fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := ArchiveDatabase.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return nil, "", fmt.Errorf("%s", ErrHandleInvalid)
	}
	// 句柄存在但实体不可读：移除失效句柄并返回统一提示
	if abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath)); res.BlobPath != "" && !fileExists(abs) {
		_ = ArchiveDatabase.DeleteResource(res.ID)
		return nil, "", fmt.Errorf("%s", ErrFileLost)
	}
	sections, err := ArchiveDatabase.ListSections(res.ID)
	if err != nil {
		return nil, "", err
	}
	// 段落表为空但 blob 在位：现场按标题解析并回填（迁移/历史资源首次访问时补建）
	if len(sections) == 0 && res.Kind == KindDocument && res.State == StateActive {
		abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath))
		if data, err := os.ReadFile(abs); err == nil {
			drafts := ParseMarkdownSections(stripBOM(string(data)))
			if len(drafts) > 0 {
				if err := ArchiveDatabase.ReplaceSections(res.ID, drafts); err == nil {
					sections, _ = ArchiveDatabase.ListSections(res.ID)
				}
			}
		}
	}
	return sections, res.Handle, nil
}

// ResourceResolve 按句柄解析资源目录条目（前端按句柄定位文件时使用）
//
// 只解析目录行，不读取正文；调用方可用返回的 BlobPath/Name 定位实际文件。
func ResourceResolve(rawHandle string) (ArchiveResource, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return ArchiveResource{}, err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok {
		return ArchiveResource{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := ArchiveDatabase.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return ArchiveResource{}, fmt.Errorf("%s", ErrHandleInvalid)
	}
	if abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath)); res.BlobPath != "" && !fileExists(abs) {
		_ = ArchiveDatabase.DeleteResource(res.ID)
		return ArchiveResource{}, fmt.Errorf("%s", ErrFileLost)
	}
	return res, nil
}

// ResourceList 列出资源目录（kind 为空表示全部）
func ResourceList(kind string, limit int) ([]ArchiveResource, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return nil, err
	}
	return ArchiveDatabase.ListResources(kind, limit)
}

// ResourceSearch 跨资源搜索
func ResourceSearch(query, kind string, limit int) ([]ArchiveSearchHit, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return nil, err
	}
	return ArchiveDatabase.SearchResources(query, kind, limit)
}

// ResourceHandle 由类型码与序号构造规范句柄（供 Go 侧调用方使用）
func ResourceHandle(kind string, seq int64) string {
	return FormatHandle(kind, seq)
}

// ResourceGetByHash 按类型码与内容哈希解析资源
func ResourceGetByHash(kind, hash string) (ArchiveResource, bool) {
	if err := EnsureArchiveInitialized(); err != nil {
		return ArchiveResource{}, false
	}
	return ArchiveDatabase.FindResourceByHash(kind, hash)
}

// ResourceSetSummary 写入资源摘要/描述（图片视觉描述、文档概要）
func ResourceSetSummary(id int64, summary string, tags []string) error {
	if err := EnsureArchiveInitialized(); err != nil {
		return err
	}
	res, found := ArchiveDatabase.GetResourceByID(id)
	if !found {
		return fmt.Errorf("资源不存在: %d", id)
	}
	if tags == nil {
		tags = res.Tags
	}
	return ArchiveDatabase.UpdateResourceExtras(id, res.Title, summary, tags)
}

// ResourceMarkLost 标记资源 blob 已丢失（GC 回收后调用）
func ResourceMarkLost(id int64) error {
	if err := EnsureArchiveInitialized(); err != nil {
		return err
	}
	return ArchiveDatabase.MarkState(id, StateLost)
}

// ResourceStats 目录统计
func ResourceStats() (ArchiveStats, error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return ArchiveStats{}, err
	}
	return ArchiveDatabase.Stats()
}

// ResourceReplaceSections 全量替换资源段落表（阅读者入库与迁移重建使用）
func ResourceReplaceSections(id int64, drafts []ArchiveSectionDraft) error {
	if err := EnsureArchiveInitialized(); err != nil {
		return err
	}
	return ArchiveDatabase.ReplaceSections(id, drafts)
}

// ResourceDelete 显式删除资源（目录行 + blob 一并移除）
func ResourceDelete(rawHandle string) error {
	if err := EnsureArchiveInitialized(); err != nil {
		return err
	}
	handle, ok := ParseHandle(rawHandle)
	if !ok {
		return fmt.Errorf("%s", ErrHandleInvalid)
	}
	res, found := ArchiveDatabase.GetResourceByHandle(handle.Kind, handle.Seq)
	if !found {
		return fmt.Errorf("%s", ErrHandleInvalid)
	}
	if res.BlobPath != "" {
		abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(res.BlobPath))
		// 显式删除：仅当没有其它目录行引用同一实体时才移除物理文件（共享实体保留）
		referenced, refErr := ArchiveDatabase.blobReferencedByOthers(res.BlobPath, res.ID)
		if refErr == nil && !referenced {
			if !fileExists(abs) {
				_ = ArchiveDatabase.DeleteResource(res.ID)
				return fmt.Errorf("%s", ErrFileLost)
			}
			_ = os.Remove(abs)
		}
	}
	return ArchiveDatabase.DeleteResource(res.ID)
}
