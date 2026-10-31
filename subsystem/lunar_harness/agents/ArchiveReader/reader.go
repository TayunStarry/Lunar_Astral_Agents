package ArchiveReader

// reader.go 归档解析入口：压缩包文件结构查询、指定条目内容读取与确定性摘要。
//
// 能力边界：只读不写。压缩包创建 / 删除 / 新增 / 移除属于归档操作智能体；
// 本包通过 Lunar Harness 原生 zip 机制（engine/persistence）读取，不自行实现压缩格式。

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/media"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// IsArchiveExt 是否为支持的压缩包扩展名
func IsArchiveExt(ext string) bool {
	return archiveExts[strings.ToLower(strings.TrimSpace(ext))]
}

// IsGGUFExt 是否为 GGUF 模型文件
func IsGGUFExt(ext string) bool {
	return strings.EqualFold(strings.TrimSpace(ext), ".gguf")
}

// ResolveFilePath 把 /document/…、相对 LocalDir 或 LocalDir 内绝对路径解析为绝对路径与相对路径
func ResolveFilePath(target string) (abs, rel string, err error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", "", fmt.Errorf("路径不能为空")
	}
	if strings.HasPrefix(trimmed, "@") {
		return "", "", fmt.Errorf("资源句柄请使用 ResolveArchivePath")
	}
	switch {
	case trimmed == "/document" || strings.HasPrefix(trimmed, "/document/"):
		sub := strings.TrimPrefix(trimmed, "/document")
		sub = strings.TrimPrefix(sub, "/")
		abs = filepath.Clean(filepath.Join(config.LocalDir, "documents", filepath.FromSlash(sub)))
	case strings.HasPrefix(trimmed, "/"):
		return "", "", fmt.Errorf("请使用 /document/ 前缀或相对 LocalDir 的路径")
	default:
		path := filepath.FromSlash(trimmed)
		if filepath.IsAbs(path) {
			abs = filepath.Clean(path)
		} else {
			abs = filepath.Clean(filepath.Join(config.LocalDir, path))
		}
	}
	if !media.IsWithinLocalDir(abs) {
		return "", "", fmt.Errorf("路径越出本地目录范围: %s", target)
	}
	relPath, relErr := filepath.Rel(filepath.Clean(config.LocalDir), abs)
	if relErr != nil {
		return "", "", fmt.Errorf("计算相对路径失败: %w", relErr)
	}
	return abs, filepath.ToSlash(relPath), nil
}

// ResolveExisting 解析句柄或路径为本地实体（不校验扩展名）
func ResolveExisting(target string) (abs, rel string, err error) {
	trimmed := strings.TrimSpace(target)
	if strings.HasPrefix(trimmed, "@") {
		res, resolveErr := resource.Resolve(trimmed)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		if res.BlobPath == "" {
			return "", "", fmt.Errorf("%s 没有本地实体", trimmed)
		}
		rel = filepath.ToSlash(res.BlobPath)
		abs = filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	} else {
		abs, rel, err = ResolveFilePath(trimmed)
		if err != nil {
			return "", "", err
		}
	}
	info, statErr := os.Stat(abs)
	if statErr != nil {
		return "", "", fmt.Errorf("文件不存在: %w", statErr)
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("%s 是目录，不是文件", target)
	}
	return abs, rel, nil
}

// ResolveArchivePath 解析压缩包目标（支持句柄与路径），并校验扩展名与实体存在性
func ResolveArchivePath(target string) (abs, rel string, err error) {
	abs, rel, err = ResolveExisting(target)
	if err != nil {
		return "", "", err
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if !IsArchiveExt(ext) {
		return "", "", fmt.Errorf("不是支持的压缩包（%s），仅支持 .zip / .ltpx", ext)
	}
	return abs, rel, nil
}

// ListEntries 查询压缩包内文件结构（不读取条目内容）
func ListEntries(target string) ([]persistence.ZipEntryInfo, error) {
	abs, _, err := ResolveArchivePath(target)
	if err != nil {
		return nil, err
	}
	return persistence.ReadZipFileList(abs)
}

// ReadEntry 读取指定条目的文本内容（按字符与字节上限截断）
func ReadEntry(target, entryName string, maxChars int) (string, error) {
	abs, _, err := ResolveArchivePath(target)
	if err != nil {
		return "", err
	}
	reader, err := zip.OpenReader(abs)
	if err != nil {
		return "", fmt.Errorf("打开压缩包失败: %w", err)
	}
	defer reader.Close()

	wanted := normalizeEntry(entryName)
	if wanted == "" {
		return "", fmt.Errorf("条目名称不能为空")
	}
	for _, file := range reader.File {
		if normalizeEntry(file.Name) != wanted {
			continue
		}
		if file.FileInfo().IsDir() {
			return "", fmt.Errorf("%s 是目录，不是文件", entryName)
		}
		rc, openErr := file.Open()
		if openErr != nil {
			return "", fmt.Errorf("打开条目失败: %w", openErr)
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxEntryReadBytes+1))
		rc.Close()
		if readErr != nil {
			return "", fmt.Errorf("读取条目失败: %w", readErr)
		}
		truncated := len(data) > maxEntryReadBytes
		if truncated {
			data = data[:maxEntryReadBytes]
		}
		text := strings.TrimPrefix(string(data), "\ufeff")
		if looksBinary(text) {
			return "", fmt.Errorf("条目 %s 是二进制内容，不做文本读取", entryName)
		}
		if maxChars <= 0 || maxChars > maxEntryReadChars {
			maxChars = maxEntryReadChars
		}
		note := ""
		if runes := []rune(text); len(runes) > maxChars {
			text = string(runes[:maxChars]) + "…"
			note = fmt.Sprintf("\n（已按 %d 字上限截断）", maxChars)
		}
		if truncated {
			note += "\n（已按字节上限截断）"
		}
		return fmt.Sprintf("【%s】\n%s%s", wanted, text, note), nil
	}
	return "", fmt.Errorf("压缩包内未找到条目 %s；可先用 %s 查看结构", entryName, ToolList)
}

// Summary 返回压缩包结构的确定性摘要（不经过模型）
func Summary(target string) (string, error) {
	abs, rel, err := ResolveArchivePath(target)
	if err != nil {
		return "", err
	}
	entries, err := persistence.ReadZipFileList(abs)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(abs)
	var zipSize int64
	if statErr == nil {
		zipSize = info.Size()
	}
	files, dirs, totalSize := 0, 0, int64(0)
	for _, entry := range entries {
		if entry.IsDir {
			dirs++
			continue
		}
		files++
		totalSize += entry.Size
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "【压缩包】%s\n", rel)
	fmt.Fprintf(&builder, "条目：%d（文件 %d / 目录 %d）｜解压后 %d 字节｜压缩包 %d 字节\n", len(entries), files, dirs, totalSize, zipSize)
	limit := len(entries)
	if limit > maxListEntries {
		limit = maxListEntries
	}
	preview := limit
	if preview > maxPreviewEntries {
		preview = maxPreviewEntries
	}
	for _, entry := range entries[:preview] {
		if entry.IsDir {
			fmt.Fprintf(&builder, "  [目录] %s\n", entry.Name)
			continue
		}
		fmt.Fprintf(&builder, "  %s（%d 字节）\n", entry.Name, entry.Size)
	}
	if preview < len(entries) {
		fmt.Fprintf(&builder, "  （其余 %d 项未展开）\n", len(entries)-preview)
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}

// Inspect 按句柄 / 路径做一次确定性解析：GGUF 走元数据，压缩包走结构或指定条目。
func Inspect(target, request string) (string, error) {
	request = strings.TrimSpace(request)
	// 句柄本身不含扩展名：先解析成本地实体再判定类型
	ext := ""
	if strings.HasPrefix(strings.TrimSpace(target), "@") {
		abs, _, err := ResolveExisting(target)
		if err != nil {
			return "", err
		}
		ext = filepath.Ext(abs)
	} else {
		ext = filepath.Ext(target)
	}
	if IsGGUFExt(ext) {
		return GGUFSummary(target)
	}
	if !IsArchiveExt(ext) {
		return "", fmt.Errorf("不是压缩包或 GGUF 模型: %s", target)
	}
	summary, err := Summary(target)
	if err != nil {
		return "", err
	}
	if request == "" {
		return summary, nil
	}
	entries, listErr := ListEntries(target)
	if listErr != nil {
		return summary, nil
	}
	if matched := matchEntry(entries, request); matched != "" {
		content, readErr := ReadEntry(target, matched, maxEntryReadChars)
		if readErr != nil {
			return summary + "\n\n" + readErr.Error(), nil
		}
		return summary + "\n\n【按需求读取】\n" + content, nil
	}
	return summary, nil
}

// matchEntry 按需求文本粗略匹配条目名（命中文件名或去扩展名后的名字）
func matchEntry(entries []persistence.ZipEntryInfo, request string) string {
	lower := strings.ToLower(request)
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := strings.ToLower(entry.Name)
		base := strings.ToLower(filepath.Base(entry.Name))
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if strings.Contains(lower, base) || strings.Contains(lower, name) || (stem != "" && strings.Contains(lower, stem)) {
			return entry.Name
		}
	}
	return ""
}

// normalizeEntry 规整条目名（统一 / 分隔并去掉首尾空白）
func normalizeEntry(name string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(name)), "/")
}

// looksBinary 采样中是否出现 NUL 字节
func looksBinary(text string) bool {
	sample := text
	if len(sample) > 4096 {
		sample = sample[:4096]
	}
	return strings.IndexByte(sample, 0) >= 0
}
