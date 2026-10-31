package ArchiveOps

// zipbuild.go 压缩包重建与打包：读取全部条目、写回 zip 字节、按来源路径打包。

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// readArchiveEntries 读取压缩包全部条目到内存（受条目数与总量双上限保护）
func readArchiveEntries(abs string) ([]zipEntry, error) {
	reader, err := zip.OpenReader(abs)
	if err != nil {
		return nil, fmt.Errorf("打开压缩包失败: %w", err)
	}
	defer reader.Close()
	if len(reader.File) > maxArchiveEntries {
		return nil, fmt.Errorf("压缩包条目过多（%d），超过上限 %d", len(reader.File), maxArchiveEntries)
	}
	entries := make([]zipEntry, 0, len(reader.File))
	total := int64(0)
	for _, file := range reader.File {
		info := file.FileInfo()
		if info.IsDir() {
			entries = append(entries, zipEntry{Name: normalizeEntry(file.Name), Dir: true, Mode: info.Mode(), ModTime: info.ModTime()})
			continue
		}
		if total+info.Size() > maxArchiveBytes {
			return nil, fmt.Errorf("压缩包解压总量超过上限 %d 字节", maxArchiveBytes)
		}
		rc, openErr := file.Open()
		if openErr != nil {
			return nil, fmt.Errorf("打开条目 %s 失败: %w", file.Name, openErr)
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取条目 %s 失败: %w", file.Name, readErr)
		}
		total += int64(len(data))
		entries = append(entries, zipEntry{Name: normalizeEntry(file.Name), Data: data, Mode: info.Mode(), ModTime: info.ModTime()})
	}
	return entries, nil
}

// writeArchiveEntries 把内存条目写回 zip 字节
func writeArchiveEntries(entries []zipEntry) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		if entry.Dir {
			header.Name = strings.TrimSuffix(entry.Name, "/") + "/"
			header.Method = zip.Store
		}
		if !entry.ModTime.IsZero() {
			header.Modified = entry.ModTime
		}
		part, err := writer.CreateHeader(header)
		if err != nil {
			return nil, fmt.Errorf("创建条目 %s 失败: %w", entry.Name, err)
		}
		if entry.Dir {
			continue
		}
		if _, err := part.Write(entry.Data); err != nil {
			return nil, fmt.Errorf("写入条目 %s 失败: %w", entry.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("关闭压缩包失败: %w", err)
	}
	return buffer.Bytes(), nil
}

// buildZipFromSources 按「相对 /document」的条目名打包（不保留 documents/ 前缀）
func buildZipFromSources(relSources []string) ([]byte, error) {
	entries := make([]zipEntry, 0, len(relSources)*4)
	total := int64(0)
	appendFile := func(entryName, abs string) error {
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", entryName, err)
		}
		total += int64(len(data))
		if total > maxArchiveBytes {
			return fmt.Errorf("打包总量超过上限 %d 字节", maxArchiveBytes)
		}
		entries = append(entries, zipEntry{Name: entryName, Data: data, Mode: 0o644})
		if len(entries) > maxArchiveEntries {
			return fmt.Errorf("打包条目超过上限 %d", maxArchiveEntries)
		}
		return nil
	}
	for _, rel := range relSources {
		abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("打包来源不存在: %s: %w", rel, err)
		}
		baseName := archiveEntryName(rel)
		if !info.IsDir() {
			if err := appendFile(baseName, abs); err != nil {
				return nil, err
			}
			continue
		}
		walkErr := filepath.WalkDir(abs, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relPath, relErr := filepath.Rel(abs, path)
			if relErr != nil {
				return relErr
			}
			entryName := baseName
			if relPath != "." {
				entryName = filepath.ToSlash(filepath.Join(baseName, relPath))
			}
			if entry.IsDir() {
				if relPath != "." {
					entries = append(entries, zipEntry{Name: entryName, Dir: true})
					if len(entries) > maxArchiveEntries {
						return fmt.Errorf("打包条目超过上限 %d", maxArchiveEntries)
					}
				}
				return nil
			}
			return appendFile(entryName, path)
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("打包来源为空")
	}
	return writeArchiveEntries(entries)
}

// archiveEntryName 去掉 documents/ 前缀，让压缩包内路径与 /document 视图一致
func archiveEntryName(rel string) string {
	cleaned := filepath.ToSlash(strings.TrimSpace(rel))
	return strings.TrimPrefix(cleaned, "documents/")
}
