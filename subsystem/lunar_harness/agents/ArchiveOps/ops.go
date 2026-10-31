package ArchiveOps

// ops.go 归档操作智能体的全量生命周期能力：
// 创建压缩包、删除压缩包、压缩包内新增文件、压缩包内移除文件。
//
// 写入统一经 Lunar Harness 原生 zip 机制与 persistence（句柄登记 / 原子落盘）；
// 结构查询与条目读取转调归档解析智能体，避免两套读取实现。
// 压缩包重建与路径处理见 zipbuild.go / helpers.go。

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// Create 创建压缩包：sources 为 /document/ 或相对 LocalDir 的文件 / 目录路径
func Create(target string, sources []string) Result {
	if strings.TrimSpace(target) == "" {
		return failResult(OpCreate, target, fmt.Errorf("压缩包路径不能为空"))
	}
	if len(sources) == 0 {
		return failResult(OpCreate, target, fmt.Errorf("请至少选择一个要打包的文件或目录"))
	}
	_, rel, err := ArchiveReader.ResolveFilePath(target)
	if err != nil {
		return failResult(OpCreate, target, err)
	}
	if ext := strings.ToLower(filepath.Ext(rel)); ext != ".zip" && ext != ".ltpx" {
		return failResult(OpCreate, target, fmt.Errorf("压缩包仅支持 .zip / .ltpx 后缀，当前为 %s", ext))
	}
	// 产物统一落在 unknown/：压缩包属不可读二进制，不进 documents/
	rel = archiveOutputRel(rel)
	relSources := make([]string, 0, len(sources))
	for _, source := range sources {
		_, sourceRel, resolveErr := ArchiveReader.ResolveFilePath(source)
		if resolveErr != nil {
			return failResult(OpCreate, target, fmt.Errorf("打包来源 %s 无效: %w", source, resolveErr))
		}
		relSources = append(relSources, sourceRel)
	}
	data, err := buildZipFromSources(relSources)
	if err != nil {
		return failResult(OpCreate, target, err)
	}
	saved, err := persistence.SaveFileEx(rel, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data))
	if err != nil {
		return failResult(OpCreate, target, err)
	}
	entries, _ := persistence.ReadZipFileList(saved.Path)
	handles := []string{}
	if saved.Handle != "" {
		handles = append(handles, saved.Handle)
	}
	finalRel := saved.RelPath
	if finalRel == "" {
		finalRel = rel
	}
	logger.SubInfo(Module, "Create", "已创建压缩包 %s（%d 条目 / %d 字节）", finalRel, len(entries), len(data))
	return okResult(OpCreate, finalRel,
		fmt.Sprintf("创建压缩包 %s（%d 个条目 / %d 字节）", finalRel, len(entries), len(data)),
		handles, len(entries), map[string]any{"zip_size": len(data), "source_count": len(sources)})
}

// Delete 删除压缩包（同时清理句柄映射）
func Delete(target string) Result {
	_, rel, err := ArchiveReader.ResolveArchivePath(target)
	if err != nil {
		return failResult(OpDelete, target, err)
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	if err := os.Remove(abs); err != nil {
		return failResult(OpDelete, target, fmt.Errorf("删除压缩包失败: %w", err))
	}
	resource.RemoveHandlesForPath(rel)
	logger.SubInfo(Module, "Delete", "已删除压缩包 %s", rel)
	return okResult(OpDelete, rel, "删除压缩包 "+rel, nil, 0, nil)
}

// AddEntry 在压缩包内新增 / 覆盖一个文件；sourcePath 非空时从本地文件读入，否则使用 content
func AddEntry(target, entryName, sourcePath, content string) Result {
	abs, rel, err := ArchiveReader.ResolveArchivePath(target)
	if err != nil {
		return failResult(OpAdd, target, err)
	}
	name, err := normalizeEntryName(entryName)
	if err != nil {
		return failResult(OpAdd, target, err)
	}
	data, err := entryContent(sourcePath, content)
	if err != nil {
		return failResult(OpAdd, target, err)
	}
	entries, err := readArchiveEntries(abs)
	if err != nil {
		return failResult(OpAdd, target, err)
	}
	replaced := false
	for index := range entries {
		if entries[index].Name == name {
			entries[index].Data = data
			entries[index].Dir = false
			entries[index].Mode = 0o644
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, zipEntry{Name: name, Data: data, Mode: 0o644})
	}
	rebuilt, err := writeArchiveEntries(entries)
	if err != nil {
		return failResult(OpAdd, target, err)
	}
	if err := persistence.WriteFileAtomic(rel, rebuilt); err != nil {
		return failResult(OpAdd, target, fmt.Errorf("写入压缩包失败: %w", err))
	}
	handles := []string{}
	if handle := registerArchiveHandle(rel); handle != "" {
		handles = append(handles, handle)
	}
	action := "新增"
	if replaced {
		action = "覆盖"
	}
	logger.SubInfo(Module, "Add", "已%s条目 %s（%d 字节）→ %s", action, name, len(data), rel)
	return okResult(OpAdd, rel,
		fmt.Sprintf("%s条目 %s（%d 字节）→ %s", action, name, len(data), rel),
		handles, len(entries), map[string]any{"entry": name, "bytes": len(data), "replaced": replaced})
}

// RemoveEntry 从压缩包内移除一个文件
func RemoveEntry(target, entryName string) Result {
	abs, rel, err := ArchiveReader.ResolveArchivePath(target)
	if err != nil {
		return failResult(OpRemove, target, err)
	}
	name, err := normalizeEntryName(entryName)
	if err != nil {
		return failResult(OpRemove, target, err)
	}
	entries, err := readArchiveEntries(abs)
	if err != nil {
		return failResult(OpRemove, target, err)
	}
	kept := make([]zipEntry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if entry.Name == name {
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !removed {
		return failResult(OpRemove, target, fmt.Errorf("压缩包内未找到条目 %s", name))
	}
	rebuilt, err := writeArchiveEntries(kept)
	if err != nil {
		return failResult(OpRemove, target, err)
	}
	if err := persistence.WriteFileAtomic(rel, rebuilt); err != nil {
		return failResult(OpRemove, target, fmt.Errorf("写入压缩包失败: %w", err))
	}
	handles := []string{}
	if handle := registerArchiveHandle(rel); handle != "" {
		handles = append(handles, handle)
	}
	logger.SubInfo(Module, "Remove", "已移除条目 %s → %s（剩余 %d 条）", name, rel, len(kept))
	return okResult(OpRemove, rel,
		fmt.Sprintf("移除条目 %s → %s（剩余 %d 个条目）", name, rel, len(kept)),
		handles, len(kept), map[string]any{"entry": name})
}

// List 转调归档解析智能体查询压缩包结构
func List(target string) ([]persistence.ZipEntryInfo, error) {
	return ArchiveReader.ListEntries(target)
}

// Read 转调归档解析智能体读取压缩包条目内容
func Read(target, entryName string, maxChars int) (string, error) {
	return ArchiveReader.ReadEntry(target, entryName, maxChars)
}
