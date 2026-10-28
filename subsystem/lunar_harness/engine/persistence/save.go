package persistence

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SaveFileEx 保存文件到 LocalDir 内，并在落盘后交给句柄系统登记
//
// 本函数只负责「把字节写到目标路径」这一件事；句柄分配、内容哈希去重、
// 同哈希实体收敛都由 engine/resource 的落盘观察者在 NotifyWrite 中完成。
// 因此返回值里的 RelPath / Handle 以观察者裁决后的最终保留路径为准：
// 若同内容已存在于优先级更高的目录，新写入的重复件会被删除，返回既有路径。
func SaveFileEx(fileName string, opts SaveOptions, body io.Reader) (SaveResult, error) {
	if fileName == "" {
		return SaveResult{}, fmt.Errorf("缺少文件名")
	}
	rel := normalizeRel(fileName)
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return SaveResult{}, fmt.Errorf("无效的文件名")
	}
	localDir := filepath.Clean(config.LocalDir)
	fullPath := filepath.Clean(filepath.Join(localDir, filepath.FromSlash(rel)))
	if fullPath != localDir && !strings.HasPrefix(fullPath, localDir+string(os.PathSeparator)) {
		return SaveResult{}, fmt.Errorf("访问被拒绝")
	}

	lock := GetFileLock(fullPath)
	lock.Lock()
	defer lock.Unlock()

	rel, fullPath = applyVersion(rel, fullPath, opts.Overwrite)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return SaveResult{}, fmt.Errorf("创建目录失败: %w", err)
	}
	file, err := os.Create(fullPath)
	if err != nil {
		return SaveResult{}, fmt.Errorf("创建文件失败: %w", err)
	}
	size, copyErr := io.Copy(file, body)
	if copyErr != nil {
		file.Close()
		return SaveResult{}, fmt.Errorf("保存文件失败: %w", copyErr)
	}
	if err := file.Sync(); err != nil {
		logger.SubError("Persistence", "Save", "同步失败: %s, %v", fullPath, err)
	}
	if err := file.Close(); err != nil {
		return SaveResult{}, fmt.Errorf("保存文件失败: %w", err)
	}

	// 落盘观察者：三棵内容仓库树内的写入自动登记句柄并做内容哈希唯一裁决
	canonicalRel, handle := NotifyWrite(rel)
	if canonicalRel == "" {
		canonicalRel = rel
	}
	if canonicalRel != rel {
		logger.SubInfo("Persistence", "Save", "内容哈希命中，复用既有路径: %s", canonicalRel)
	}
	// 哈希冲突时新文件可能已被删除，返回值必须指向最终保留的实体
	canonicalAbs := filepath.Join(localDir, filepath.FromSlash(canonicalRel))
	canonicalSize := size
	if canonicalRel != rel {
		if info, statErr := os.Stat(canonicalAbs); statErr == nil {
			canonicalSize = info.Size()
		}
	}
	logger.SubInfo("Persistence", "Save", "成功保存文件: %s, 覆盖: %t, 句柄: %s, 保留路径: %s",
		fullPath, opts.Overwrite, handle, canonicalRel)
	return SaveResult{
		Name:    filepath.Base(canonicalRel),
		Path:    canonicalAbs,
		RelPath: canonicalRel,
		Handle:  handle,
		Deduped: canonicalRel != rel,
		Size:    canonicalSize,
	}, nil
}

// SaveFile 兼容既有调用方（覆盖语义由 overwrite 控制）
func SaveFile(fileName string, overwrite bool, body io.Reader) (string, string, error) {
	result, err := SaveFileEx(fileName, SaveOptions{Overwrite: overwrite}, body)
	if err != nil {
		return "", "", err
	}
	return result.Name, result.Path, nil
}

// applyVersion 目标已存在且不覆盖时，追加时间戳生成新版本路径
func applyVersion(rel, fullPath string, overwrite bool) (string, string) {
	if overwrite {
		return rel, fullPath
	}
	if _, err := os.Stat(fullPath); err != nil {
		return rel, fullPath
	}
	timestamp := time.Now().Format("20060102-150405")
	ext := filepath.Ext(rel)
	base := strings.TrimSuffix(rel, ext)
	rel = base + "_" + timestamp + ext
	return rel, filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
}

// DecodeFileName 解码base64编码的文件名
func DecodeFileName(encodedName string) (string, error) {
	if encodedName == "" {
		return "", fmt.Errorf("缺少文件名")
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedName)
	if err != nil {
		return "", fmt.Errorf("文件名解码错误: %w", err)
	}
	fileName := string(decodedBytes)
	if fileName == "" {
		return "", fmt.Errorf("文件名解码后为空")
	}
	return fileName, nil
}

// GetFileLock 获取指定文件路径的互斥锁
func GetFileLock(filePath string) *sync.Mutex {
	lock, _ := FileLocks.LoadOrStore(filePath, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
