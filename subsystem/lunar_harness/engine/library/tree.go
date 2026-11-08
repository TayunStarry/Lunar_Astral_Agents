package library

// tree.go 递归清单：一次返回 4 个存储根下的完整目录结构。

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ListAll 递归扫描 4 个存储根，返回扁平条目列表（前端按路径自行组装层级）。
// database 不是句柄存储根，但资源库需要浏览其中的 .db 文件，因此额外补一个顶层目录条目。
func ListAll() Tree {
	entries := make([]Entry, 0, 256)
	truncated := false
	for _, root := range Roots {
		if len(entries) >= maxTreeEntries {
			truncated = true
			break
		}
		walkRoot(root, &entries, &truncated)
	}
	if len(entries) < maxTreeEntries {
		if info, err := os.Stat(absOf(dbDirRel)); err == nil && info.IsDir() {
			entries = append(entries, Entry{
				Root:  dbDirRel,
				Path:  dbDirRel,
				Name:  dbDirRel,
				Dir:   true,
				MTime: info.ModTime().Unix(),
			})
			walkDatabase(&entries, &truncated)
		}
	} else {
		truncated = true
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return Tree{Roots: append([]string(nil), Roots...), Entries: entries, Truncated: truncated}
}

// walkDatabase 扁平扫描 database/ 下的数据库文件。
// 不输出中间目录、不保留层级：显示名取相对 database 的路径（如 bilibili/viewer.db），
// 既能让同名库互相区分，也保证路径仍是唯一键；排序交给 ListAll 的路径排序。
func walkDatabase(entries *[]Entry, truncated *bool) {
	start := absOf(dbDirRel)
	err := filepath.WalkDir(start, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil {
			return nil
		}
		if len(*entries) >= maxTreeEntries {
			*truncated = true
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(localDir(), path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		// 只展示可预览的库文件，跳过 -wal / -shm / -journal 与索引等伴随文件。
		if !isDBFile(rel) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		*entries = append(*entries, Entry{
			Root:  dbDirRel,
			Path:  rel,
			Name:  strings.TrimPrefix(rel, dbDirRel+"/"),
			Dir:   false,
			Size:  info.Size(),
			MTime: info.ModTime().Unix(),
			Ext:   extOf(rel),
		})
		return nil
	})
	_ = err
}

// walkRoot 遍历单个存储根。
func walkRoot(root string, entries *[]Entry, truncated *bool) {
	start := absOf(root)
	err := filepath.WalkDir(start, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil {
			return nil
		}
		if len(*entries) >= maxTreeEntries {
			*truncated = true
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(localDir(), path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == root {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		entry := Entry{
			Root:  root,
			Path:  rel,
			Name:  d.Name(),
			Dir:   d.IsDir(),
			Size:  info.Size(),
			MTime: info.ModTime().Unix(),
		}
		if !entry.Dir {
			entry.Ext = extOf(rel)
		}
		*entries = append(*entries, entry)
		return nil
	})
	_ = err
}
