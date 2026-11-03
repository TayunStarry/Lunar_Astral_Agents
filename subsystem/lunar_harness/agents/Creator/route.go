package Creator

// route.go 任务自动路由：按目标路径的扩展名与实际规模（目录文件数 / 总字节 / 最大文件）
// 选择承接该操作的智能体，不依赖调用方自觉传参。
//
//   - 压缩包 / GGUF 读取 → 归档解析（ArchiveReader）
//   - 压缩包写入         → 归档操作（ArchiveOps）
//   - 大型文件夹 / 大文件读取 → 文献解析（DocumentReader）
//   - 文本文件读写       → 文件编辑（FileEditor）

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

// routeKind 承接操作的智能体已在 type.go 定义。

const (
	// routeFileEditor 文件编辑
	routeFileEditor routeKind = "file_editor"
	// routeArchiveReader 归档解析
	routeArchiveReader routeKind = "archive_reader"
	// routeArchiveOps 归档操作
	routeArchiveOps routeKind = "archive_ops"
	// routeDocumentReader 文献解析
	routeDocumentReader routeKind = "document_reader"
)

// routeTarget 按目标实际状态选择承接智能体。
func routeTarget(target, op string) (routeDecision, error) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return routeDecision{}, fmt.Errorf("目标路径不能为空")
	}
	archiveTarget, entry := splitArchiveEntry(raw)
	resolveTarget := raw
	if archiveTarget != "" {
		resolveTarget = archiveTarget
	}
	abs, rel, err := resolveActualPath(resolveTarget)
	if err != nil {
		return routeDecision{}, err
	}
	decision := routeDecision{Abs: abs, Rel: rel, Entry: entry}

	// 先按实际实体判定目录：目录名恰好以 .zip 结尾时仍按目录处理
	if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
		return routeFolderDecision(decision, op, abs)
	}

	ext := strings.ToLower(filepath.Ext(abs))
	archive := ArchiveReader.IsArchiveExt(ext)
	gguf := ArchiveReader.IsGGUFExt(ext)
	if archive || gguf {
		return routeArchiveDecision(decision, op, archive, gguf)
	}

	info, statErr := os.Stat(abs)
	switch {
	case os.IsNotExist(statErr):
		// 创建类操作：按文本文件处理
		decision.Kind = routeFileEditor
		decision.Reason = "目标尚不存在，按文本文件处理"
		return decision, nil
	case statErr != nil:
		return routeDecision{}, fmt.Errorf("读取目标状态失败: %w", statErr)
	}

	if fileflow.Mutating(op) {
		decision.Kind = routeFileEditor
		decision.Reason = "文本文件写入"
		return decision, nil
	}
	if info.Size() > largeFileBytes {
		decision.Kind = routeDocumentReader
		decision.Large = true
		decision.Reason = fmt.Sprintf("文件 %d 字节，超过单次直读上限", info.Size())
		return decision, nil
	}
	decision.Kind = routeFileEditor
	decision.Reason = fmt.Sprintf("文本文件读取（%d 字节）", info.Size())
	return decision, nil
}

// resolveActualPath 把目标解析为实际实体路径：
// 严格路径存在时直接用；不存在时交给统一文件定位器做同名 / 后缀匹配
// （压缩包产物落在 unknown/，文件编辑自身也会用同样的模糊定位）。
func resolveActualPath(target string) (abs, rel string, err error) {
	abs, rel, err = fileflow.Resolve(target)
	if err != nil {
		return "", "", err
	}
	if _, statErr := os.Stat(abs); os.IsNotExist(statErr) {
		match, locateErr := resourcehub.Resolve(target, nil)
		if locateErr == nil {
			return match.Abs, match.Rel, nil
		}
		if _, ambiguous := locateErr.(*resourcehub.AmbiguousError); ambiguous {
			return "", "", locateErr
		}
	}
	return abs, rel, nil
}

// routeArchiveDecision 压缩包 / GGUF 的路由。
func routeArchiveDecision(decision routeDecision, op string, archive, gguf bool) (routeDecision, error) {
	if fileflow.Mutating(op) {
		if gguf {
			return routeDecision{}, fmt.Errorf("GGUF 模型只支持读取，不支持写入")
		}
		decision.Kind = routeArchiveOps
		decision.Reason = "压缩包写入"
		return decision, nil
	}
	decision.Kind = routeArchiveReader
	if gguf {
		decision.Reason = "GGUF 元数据读取"
	} else {
		decision.Reason = "压缩包结构 / 条目读取"
	}
	return decision, nil
}

// routeFolderDecision 目录的路由：大型目录读取交给文献解析，其余交给文件编辑。
func routeFolderDecision(decision routeDecision, op string, abs string) (routeDecision, error) {
	decision.IsDir = true
	if fileflow.Mutating(op) {
		decision.Kind = routeFileEditor
		decision.Reason = "目录写入由文件编辑处理（二进制容器会被拒绝）"
		return decision, nil
	}
	files, total, largest, err := scanFolderScale(abs)
	if err != nil {
		return routeDecision{}, fmt.Errorf("扫描目录规模失败: %w", err)
	}
	decision.Large = isLargeFolder(files, total, largest)
	if decision.Large {
		decision.Kind = routeDocumentReader
		decision.Reason = fmt.Sprintf("大型目录（文件 %d / 总字节 %d / 最大文件 %d）", files, total, largest)
		return decision, nil
	}
	decision.Kind = routeFileEditor
	decision.Reason = fmt.Sprintf("小型目录（文件 %d / 总字节 %d）", files, total)
	return decision, nil
}

// scanFolderScale 扫描目录的文件数、总字节与最大文件（有限流）。
func scanFolderScale(abs string) (files int, total int64, largest int64, err error) {
	scanned := 0
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		scanned++
		if scanned > maxFolderScanEntries {
			return filepath.SkipAll
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		files++
		total += info.Size()
		if info.Size() > largest {
			largest = info.Size()
		}
		return nil
	})
	return files, total, largest, err
}

// isLargeFolder 目录规模是否达到「大型」阈值。
func isLargeFolder(files int, total, largest int64) bool {
	return files > largeFolderFiles || total > largeFolderBytes || largest > largeFolderLargest
}

// splitArchiveEntry 拆分「压缩包 + 内部条目」写法（pkg.zip/entry 或 pkg.zip:entry）。
func splitArchiveEntry(path string) (archive, entry string) {
	clean := filepath.ToSlash(strings.TrimSpace(path))
	lower := strings.ToLower(clean)
	for _, marker := range []string{".zip", ".ltpx"} {
		if index := strings.Index(lower, marker+"/"); index >= 0 {
			end := index + len(marker)
			return clean[:end], clean[end+1:]
		}
		if index := strings.Index(lower, marker+":"); index >= 0 {
			end := index + len(marker)
			return clean[:end], clean[end+1:]
		}
	}
	return "", ""
}
