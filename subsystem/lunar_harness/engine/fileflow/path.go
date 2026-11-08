package fileflow

// path.go 路径解析：把 /document 相对路径、句柄、本地相对路径与绝对路径统一为
// 「本地目录内的绝对路径 + 相对本地目录的斜杠路径」，供快照与审查使用。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// nowText 当前时间文本（流程 ID 前缀）。
func nowText() string { return time.Now().Format("20060102-150405") }

// NewFlowID 生成流程 ID（时间戳 + 进程内序号）。
func NewFlowID() string {
	return fmt.Sprintf("%s-%d", nowText(), flowSeq.Add(1))
}

// Resolve 解析目标路径；文件不存在时仍返回严格路径，供创建类操作使用。
func Resolve(target string) (abs, rel string, err error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", "", fmt.Errorf("路径不能为空")
	}
	if strings.Contains(filepath.ToSlash(trimmed), "..") {
		return "", "", fmt.Errorf("路径不允许包含 ..")
	}
	localRoot := filepath.Clean(config.LocalDir)

	if strings.HasPrefix(trimmed, "@") {
		res, resolveErr := resource.Resolve(trimmed)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		if res.BlobPath == "" {
			return "", "", fmt.Errorf("%s 没有本地实体", trimmed)
		}
		rel = filepath.ToSlash(res.BlobPath)
		abs = filepath.Clean(filepath.Join(localRoot, filepath.FromSlash(rel)))
		return withinLocalDir(abs, rel)
	}

	switch {
	case trimmed == "/document" || strings.HasPrefix(trimmed, "/document/"):
		rel = "documents" + strings.TrimPrefix(trimmed, "/document")
	case filepath.IsAbs(filepath.FromSlash(trimmed)):
		abs = filepath.Clean(filepath.FromSlash(trimmed))
		relative, relErr := Relative(abs)
		if relErr != nil {
			return "", "", relErr
		}
		return withinLocalDir(abs, relative)
	case strings.HasPrefix(trimmed, "/"):
		return "", "", fmt.Errorf("请使用 /document/ 前缀或相对路径: %s", target)
	default:
		cleaned := filepath.ToSlash(trimmed)
		cleaned = strings.TrimPrefix(cleaned, "./")
		// 调用方可能直接传入「相对 LocalDir」的路径（如 local_data/documents/x）。
		// 不剥掉 LocalDir 前缀会被当成裸相对路径再补一层 documents/，快照路径会翻倍。
		if prefix := localRootPrefix(); prefix != "" {
			cleaned = strings.TrimPrefix(cleaned, prefix+"/")
		}
		// 已显式写出存储根时保持原样（压缩包产物常在 unknown/）
		for _, root := range []string{
			resource.StorageRootDocuments, resource.StorageRootAudios,
			resource.StorageRootMultimedia, resource.StorageRootUnknown,
		} {
			if strings.HasPrefix(cleaned, root+"/") {
				rel = cleaned
				break
			}
		}
		if rel == "" {
			// 裸相对路径统一落在 /document，与文件编辑的路径语义一致
			rel = resource.StorageRootDocuments + "/" + cleaned
		}
	}

	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	abs = filepath.Clean(filepath.Join(localRoot, filepath.FromSlash(rel)))
	return withinLocalDir(abs, rel)
}

// Relative 把绝对路径转回相对本地目录的斜杠路径。
//
// 比较双方都先转绝对：LocalDir 是相对路径时，直接拿相对根与绝对目标做 Rel 会失败，
// 进而把绝对路径误判成越界。
func Relative(abs string) (string, error) {
	rel, err := filepath.Rel(absLocalRoot(), absOrClean(abs))
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("路径越出本地目录: %s", abs)
	}
	return rel, nil
}

// withinLocalDir 校验路径必须落在本地目录内。
func withinLocalDir(abs, rel string) (string, string, error) {
	localRoot := absLocalRoot()
	target := absOrClean(abs)
	if target != localRoot && !strings.HasPrefix(target, localRoot+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("路径越出本地目录: %s", rel)
	}
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", "", fmt.Errorf("路径无效: %s", rel)
	}
	return abs, filepath.ToSlash(rel), nil
}

// absLocalRoot 返回 LocalDir 的绝对路径；解析失败时退回原值，保持旧行为。
func absLocalRoot() string {
	root, err := filepath.Abs(filepath.Clean(config.LocalDir))
	if err != nil {
		return filepath.Clean(config.LocalDir)
	}
	return root
}

// absOrClean 把路径统一成绝对路径（输入本身就是相对路径时按当前工作目录解析）。
func absOrClean(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// localRootPrefix 返回 LocalDir 相对当前工作目录的斜杠前缀；LocalDir 本身是绝对路径时返回空串。
func localRootPrefix() string {
	if filepath.IsAbs(config.LocalDir) {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(config.LocalDir))
	if cleaned == "." {
		return ""
	}
	return strings.TrimSuffix(cleaned, "/")
}
