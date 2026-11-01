package locator

// index.go 搜索根的文件名索引：纯文件名、相对路径的模糊定位都走这里。
//
// 索引按搜索根缓存，TTL 与显式 Invalidate 共同控制新鲜度；
// 1000+ 文件的目录只在首次查询时遍历一次，之后的查询是哈希取表。

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// dirIndex 一个搜索根的文件名索引
type dirIndex struct {
	// byBase 小写文件名 → 相对 LocalDir 的路径
	byBase map[string][]string
	// byStem 小写去扩展名文件名 → 相对 LocalDir 的路径
	byStem map[string][]string
	// built 构建时间（配合 TTL 判断新鲜度）
	built time.Time
}

// defaultRoots 未指定搜索根时使用的存储根
func defaultRoots() []string {
	return resource.StorageRoots
}

// normalizedRoots 规整搜索根列表：去空白、去首尾斜杠，空列表回落到默认根
func normalizedRoots(roots []string) []string {
	if len(roots) == 0 {
		return defaultRoots()
	}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		cleaned := strings.Trim(strings.ReplaceAll(strings.TrimSpace(root), `\`, "/"), "/")
		if cleaned != "" {
			out = append(out, cleaned)
		}
	}
	if len(out) == 0 {
		return defaultRoots()
	}
	return out
}

// indexFor 取搜索根的索引（命中缓存且未过期时直接复用）
func indexFor(root string) *dirIndex {
	rootAbs := filepath.Clean(filepath.Join(config.LocalDir, filepath.FromSlash(root)))
	indexMu.Lock()
	defer indexMu.Unlock()
	if cached, ok := indexCache[rootAbs]; ok && time.Since(cached.built) < indexTTL {
		return cached
	}
	index := buildIndex(rootAbs)
	indexCache[rootAbs] = index
	return index
}

// buildIndex 遍历搜索根，建立文件名与去扩展名文件名的索引
func buildIndex(rootAbs string) *dirIndex {
	index := &dirIndex{
		byBase: map[string][]string{},
		byStem: map[string][]string{},
		built:  time.Now(),
	}
	localDir := filepath.Clean(config.LocalDir)
	_ = filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(localDir, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		base := strings.ToLower(entry.Name())
		index.byBase[base] = append(index.byBase[base], relSlash)
		stem := strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))
		if stem != "" && stem != base {
			index.byStem[stem] = append(index.byStem[stem], relSlash)
		}
		return nil
	})
	return index
}

// Invalidate 清空索引缓存（本进程写入文件后调用，保证下一次查询立即可见）
func Invalidate() {
	indexMu.Lock()
	defer indexMu.Unlock()
	indexCache = map[string]*dirIndex{}
}
