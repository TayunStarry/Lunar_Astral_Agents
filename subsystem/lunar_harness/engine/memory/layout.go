package memory

// layout.go 记忆库磁盘布局 — 总库 + 正式簇 双轨。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// clusterDirForID 返回 ID 对应的目录绝对路径（总库 = store-all，簇 = c-<hash8>）
func clusterDirForID(baseDir, id string) string {
	return filepath.Join(baseDir, clusterDirName(id))
}

// sortedDocsByTime 按（入库时间, ID）排序，保证分片内容与顺序稳定
func sortedDocsByTime(docs []Document) []Document {
	sorted := append([]Document(nil), docs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Timestamp != sorted[j].Timestamp {
			return sorted[i].Timestamp < sorted[j].Timestamp
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

// sortedDocsFromMap 把 ID→文档映射转为稳定顺序的切片
func sortedDocsFromMap(docs map[string]Document) []Document {
	out := make([]Document, 0, len(docs))
	for _, doc := range docs {
		out = append(out, doc)
	}
	return sortedDocsByTime(out)
}

// scanMemoryLayout 扫描 baseDir 下的全部目录：
//   - store-all → 总库
//   - c-<hash8> → 正式簇
//   - 其他有 metadata.json / documents_* 的目录 → 旧布局目录（迁移后删除）
func scanMemoryLayout(baseDir string) (*diskLayout, error) {
	layout := &diskLayout{
		Docs:    make(map[string]Document),
		DocDirs: make(map[string]string),
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return layout, nil
		}
		return nil, fmt.Errorf("读取记忆库目录失败: %w", err)
	}

	var legacyDirs, clusterDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dir := filepath.Join(baseDir, name)
		if isStoreDirName(name) {
			layout.StoreDir = dir
			continue
		}
		if isClusterDirName(name) {
			clusterDirs = append(clusterDirs, dir)
			continue
		}
		if dirHasMemoryData(dir) {
			legacyDirs = append(legacyDirs, dir)
		}
	}
	sort.Strings(legacyDirs)
	sort.Strings(clusterDirs)
	layout.LegacyDirs = legacyDirs
	layout.ClusterDirs = clusterDirs

	// 搜索顺序：总库 → 正式簇 → 旧布局（总库优先保证条目归属一致）
	searchDirs := make([]string, 0)
	if layout.StoreDir != "" {
		searchDirs = append(searchDirs, layout.StoreDir)
	}
	searchDirs = append(searchDirs, clusterDirs...)
	searchDirs = append(searchDirs, legacyDirs...)

	for _, dir := range searchDirs {
		docs, _, _, err := readDocumentsFromDir(dir)
		if err != nil {
			return nil, err
		}
		layout.FileCount += countShardFiles(dir)
		for _, doc := range docs {
			if doc.ID == "" {
				continue
			}
			if _, exists := layout.Docs[doc.ID]; exists {
				continue // 总库/簇目录已先扫描到，旧目录重复时忽略
			}
			layout.Docs[doc.ID] = doc
			layout.DocDirs[doc.ID] = dir
		}
	}
	return layout, nil
}

// dirHasMemoryData 判断目录是否为记忆数据目录
func dirHasMemoryData(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "metadata.json")); err == nil {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "documents_*.json"))
	return len(matches) > 0
}

// countShardFiles 统计目录内分片文件数
func countShardFiles(dir string) int {
	docs, _ := filepath.Glob(filepath.Join(dir, "documents_*.json"))
	embs, _ := filepath.Glob(filepath.Join(dir, "embeddings_*.json"))
	return len(docs) + len(embs)
}

// layoutMatchesIndex 判断磁盘布局是否已与索引完全一致（一致时无需任何搬迁）。
func layoutMatchesIndex(baseDir string, layout *diskLayout, index *ClusterIndex) bool {
	if layout == nil || index == nil {
		return false
	}
	if len(layout.LegacyDirs) > 0 {
		return false
	}
	if layout.StoreDir == "" {
		return false
	}
	// 总库条目 + Assignments 条目 = 原始总数
	if index.StoreSize+len(index.Assignments) != len(layout.Docs) {
		return false
	}
	// 新语义：所有正式簇必须满足 Size ≥ ClusterUpgradeSize
	for _, node := range index.Clusters {
		if node.Size < ClusterUpgradeSize {
			return false
		}
	}
	// 正式簇目录全部存在且条目数与索引一致
	for _, node := range index.Clusters {
		dir := clusterDirForID(baseDir, node.ID)
		docs, _, _, err := readDocumentsFromDir(dir)
		if err != nil || len(docs) != node.Size {
			return false
		}
	}
	// 不在索引里的 c-* 目录必须为空（簇被废弃）
	for _, dir := range layout.ClusterDirs {
		name := filepath.Base(dir)
		if !isExpectedClusterDir(baseDir, index, name) {
			docs, _, _, _ := readDocumentsFromDir(dir)
			if len(docs) > 0 {
				return false // 有残留条目
			}
		}
	}
	return true
}

// isExpectedClusterDir 判断 c-* 目录是否被索引引用
func isExpectedClusterDir(baseDir string, index *ClusterIndex, dirName string) bool {
	for _, node := range index.Clusters {
		if clusterDirName(node.ID) == dirName {
			return true
		}
	}
	return false
}

// loadClusterIndexFromDisk 读取 clusters.json；不存在或版本不符返回 nil
func loadClusterIndexFromDisk(baseDir string) (*ClusterIndex, time.Time, bool) {
	path := clusterIndexPath(baseDir)
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	var index ClusterIndex
	if err := readJSONFile(path, &index); err != nil || index.Version != ClusterIndexVersion {
		return nil, info.ModTime(), false
	}
	return &index, info.ModTime(), true
}

// =============================================================================
// 核心：prepareLayout / materializeLayout
// =============================================================================

// prepareLayout 启动时幂等准备：布局与索引一致则直接加载，否则迁移/重建。
// forceRebuild 为 true 时忽略既有索引，按当前条目重新聚类。
