package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// ==== 硬链接原语 ====

// linkFile 在 linkPath 处创建指向 existingPath 的硬链接
// 等价于 mklink /H，两路径必须位于同一 NTFS 卷
func linkFile(existingPath, linkPath string) error {
	existing, err := windows.UTF16PtrFromString(existingPath)
	if err != nil {
		return err
	}
	link, err := windows.UTF16PtrFromString(linkPath)
	if err != nil {
		return err
	}
	return windows.CreateHardLink(link, existing, 0)
}

// ==== 文件身份判定 ====

// fileIdentity 文件唯一标识（卷序列号 + 文件索引），用于判断两个路径是否同一物理文件
type fileIdentity struct {
	// volume 卷序列号
	volume uint32
	// indexHigh 文件索引高 32 位
	indexHigh uint32
	// indexLow 文件索引低 32 位
	indexLow uint32
}

// fileID 获取文件的唯一标识
func fileID(path string) (fileIdentity, error) {
	var id fileIdentity
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return id, err
	}
	handle, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return id, err
	}
	defer windows.CloseHandle(handle)

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return id, err
	}
	id.volume = info.VolumeSerialNumber
	id.indexHigh = info.FileIndexHigh
	id.indexLow = info.FileIndexLow
	return id, nil
}

// sameFile 判断两个路径是否指向同一物理文件（硬链接命中判定）
func sameFile(a, b string) bool {
	ia, err := fileID(a)
	if err != nil {
		return false
	}
	ib, err := fileID(b)
	if err != nil {
		return false
	}
	return ia == ib
}

// ==== 内容处理 ====

// hashFile 流式计算文件 SHA256
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// replaceWithHardlink 将 path 处的文件替换为指向 sourcePath 的硬链接
// 先在旁路创建临时链接，再原子覆盖回原路径（os.Rename 在 Windows 上使用
// MoveFileEx 的 REPLACE_EXISTING，覆盖时删除原文件、链接数随之归位）
func replaceWithHardlink(path, sourcePath string) error {
	tmp := path + LinkTempSuffix
	os.Remove(tmp)
	if err := linkFile(sourcePath, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// storePath 由内容哈希与扩展名生成仓库内实体路径
func storePath(sourceDir, hash, ext string) string {
	return filepath.Join(sourceDir, hash+ext)
}

// walkRecords 递归枚举 root 下满足扩展名过滤的普通文件（跳过仓库目录与重解析点）
func walkRecords(root string, exts map[string]bool) ([]fileRecord, error) {
	var records []fileRecord
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			// 跳过内容仓库本身与链接目录（Junction/符号链接），防止环与重复统计
			if d.Name() == SourceDirName || d.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		// 只处理普通文件，跳过符号链接等特殊条目
		if !d.Type().IsRegular() {
			return nil
		}
		ext := extName(d.Name())
		if !exts[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		records = append(records, fileRecord{Path: path, Ext: ext, Size: info.Size()})
		return nil
	})
	return records, err
}

// extName 取小写扩展名（含点）
func extName(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return toLower(name[i:])
		}
		if name[i] == '\\' || name[i] == '/' {
			break
		}
	}
	return ""
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
