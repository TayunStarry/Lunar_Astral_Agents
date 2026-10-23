package persistence

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SaveFileEx 保存文件并维护句柄映射
//
// 去重语义：媒体类按内容哈希全局唯一。目标位于生成目录/集合子目录时优先于
// audios|multimedia 根目录；哈希已存在且既有路径优先级不低时直接复用其路径，
// 新路径更优时才落盘并迁移句柄。MustStore 只影响新内容的落点。
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
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return SaveResult{}, fmt.Errorf("创建目录失败: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(rel))
	kind := KindOfExt(ext)
	if kind == "" {
		kind = KindDocument
	}
	// 只有媒体类参与内容寻址式去重；文本类以文件名作为业务身份
	if !IsStoragePath(rel) || !IsMediaExtension(ext) {
		return saveStreamDirect(rel, fullPath, opts, body)
	}

	lock := GetFileLock(fullPath)
	lock.Lock()
	defer lock.Unlock()

	tmpPath, hash, size, err := writeTempStream(filepath.Dir(fullPath), ext, body)
	if err != nil {
		return SaveResult{}, err
	}
	// 内容哈希唯一：既有实体优先级不低时直接复用；新路径更优（如生成目录）时
	// 继续写入，由 upsertHandleWithKnownHash 迁移句柄并删除根目录旧实体。
	// 注意：MustStore 只影响「新内容」的落点，不能因此制造同哈希的第二份实体。
	if existing, ok := FindActiveResourceByHash(kind, hash); ok {
		if !preferNewPath(rel, existing.BlobPath) {
			_ = os.Remove(tmpPath)
			abs := filepath.Join(localDir, filepath.FromSlash(existing.BlobPath))
			logger.SubInfo("Persistence", "Save", "内容哈希命中，复用既有路径: %s", existing.BlobPath)
			return SaveResult{
				Name:    filepath.Base(existing.BlobPath),
				Path:    abs,
				RelPath: existing.BlobPath,
				Handle:  existing.Handle,
				Deduped: true,
				Size:    existing.SizeBytes,
			}, nil
		}
	}

	rel, fullPath = applyVersion(rel, fullPath, opts.Overwrite)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		os.Remove(tmpPath)
		return SaveResult{}, fmt.Errorf("创建目录失败: %w", err)
	}
	if err := os.Rename(tmpPath, fullPath); err != nil {
		// 跨卷等场景：退回复制
		if copyErr := copyIntoPlace(tmpPath, fullPath); copyErr != nil {
			os.Remove(tmpPath)
			return SaveResult{}, fmt.Errorf("保存文件失败: %w", copyErr)
		}
		os.Remove(tmpPath)
	}
	handle, canonicalRel, handleErr := upsertHandleWithKnownHash(rel, fullPath, hash)
	if handleErr != nil {
		logger.SubWarn("Persistence", "Save", "句柄登记失败(%s): %v", rel, handleErr)
	}
	if canonicalRel == "" {
		canonicalRel = rel
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

// saveStreamDirect 非媒体或非内容仓库路径：直接写目标文件
func saveStreamDirect(rel, fullPath string, opts SaveOptions, body io.Reader) (SaveResult, error) {
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
	handle, handleErr := UpsertHandleForPath(rel)
	if handleErr != nil {
		logger.SubWarn("Persistence", "Save", "句柄登记失败(%s): %v", rel, handleErr)
	}
	logger.SubInfo("Persistence", "Save", "成功保存文件: %s, 覆盖: %t, 句柄: %s", fullPath, opts.Overwrite, handle)
	return SaveResult{Name: filepath.Base(rel), Path: fullPath, RelPath: rel, Handle: handle, Size: size}, nil
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

// writeTempStream 把输入流写入目标目录下的临时文件，同时计算 sha16 与字节数
func writeTempStream(dir, ext string, body io.Reader) (string, string, int64, error) {
	tmp, err := os.CreateTemp(dir, "lunar-save-*"+ext)
	if err != nil {
		return "", "", 0, fmt.Errorf("创建临时文件失败: %w", err)
	}
	hasher := sha256.New()
	writer := io.MultiWriter(tmp, hasher)
	size, copyErr := io.Copy(writer, body)
	if copyErr != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", "", 0, fmt.Errorf("保存文件失败: %w", copyErr)
	}
	if err := tmp.Sync(); err != nil {
		logger.SubError("Persistence", "Save", "同步失败: %s, %v", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", "", 0, fmt.Errorf("保存文件失败: %w", err)
	}
	return tmp.Name(), hex.EncodeToString(hasher.Sum(nil))[:16], size, nil
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
