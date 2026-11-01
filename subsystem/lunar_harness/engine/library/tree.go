package library

// tree.go 递归清单：一次返回 4 个存储根下的完整目录结构。

import (
	"io/fs"
	"path/filepath"
	"sort"
)

// ListAll 递归扫描 4 个存储根，返回扁平条目列表（前端按路径自行组装层级）。
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
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return Tree{Roots: append([]string(nil), Roots...), Entries: entries, Truncated: truncated}
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
