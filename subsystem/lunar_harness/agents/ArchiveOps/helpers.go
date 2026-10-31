package ArchiveOps

// helpers.go 归档操作的路径与条目辅助函数。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// entryContent 读取新增条目的字节内容（本地文件优先，其次内联文本）
func entryContent(sourcePath, content string) ([]byte, error) {
	if strings.TrimSpace(sourcePath) != "" {
		abs, _, err := ArchiveReader.ResolveFilePath(sourcePath)
		if err != nil {
			return nil, err
		}
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			return nil, fmt.Errorf("读取来源文件失败: %w", readErr)
		}
		return data, nil
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("请提供来源文件路径或要写入的文本内容")
	}
	return []byte(content), nil
}

// normalizeEntryName 规整条目名并拒绝非法值
func normalizeEntryName(name string) (string, error) {
	cleaned := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(name)), "/")
	if cleaned == "" {
		return "", fmt.Errorf("条目名称不能为空")
	}
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("条目名称不允许包含 ..")
	}
	return cleaned, nil
}

// normalizeEntry 规整压缩包内记录名（统一 / 分隔并去掉首尾空白）
func normalizeEntry(name string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(name)), "/")
}

// archiveOutputRel 生成压缩包的统一落点：unknown/（不可读二进制根）
//
// 无论调用方给的是 /document/xxx.zip 还是裸文件名，产物都落到 unknown/，
// documents/ 只保留文本类；已显式给出 unknown/ 路径时保持原样。
func archiveOutputRel(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if strings.HasPrefix(rel, resource.StorageRootUnknown+"/") {
		return rel
	}
	return resource.StorageRootUnknown + "/" + filepath.Base(rel)
}

// registerArchiveHandle 重新登记压缩包句柄（失败时返回空串）
func registerArchiveHandle(rel string) string {
	_, handle, err := resource.RegisterPath(rel)
	if err != nil {
		return ""
	}
	return handle
}
