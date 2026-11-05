package memory

// keyword_persist.go 索引构建与持久化

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"os"
	"path/filepath"
	"sort"
)

func (d *Store) keywordIndexSources() []keywordIndexSource {
	var sources []keywordIndexSource
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		for i := range c.Documents {
			sources = append(sources, keywordIndexSource{Doc: c.Documents[i], Collection: c.Name})
		}
		c.mu.RUnlock()
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Doc.Timestamp != sources[j].Doc.Timestamp {
			return sources[i].Doc.Timestamp < sources[j].Doc.Timestamp
		}
		return sources[i].Doc.ID < sources[j].Doc.ID
	})
	return sources
}

func buildKeywordIndexFromSources(sources []keywordIndexSource) *keywordIndex {
	idx := newKeywordIndex()
	for _, src := range sources {
		idx.add(src.Doc, src.Collection)
	}
	return idx
}

// keywordIndexMatchesSources 启动对账：条目集合、类型、待处理标记与正文哈希必须一致，
// 否则视为索引过期/损坏，走全量幂等重建。
func keywordIndexMatchesSources(idx *keywordIndex, sources []keywordIndexSource) bool {
	if idx == nil || len(idx.docs) != len(sources) {
		return false
	}
	for _, src := range sources {
		kd, ok := idx.docs[src.Doc.ID]
		if !ok {
			return false
		}
		if kd.Type != documentType(src.Doc) || kd.Hash != keywordContentHash(src.Doc.Content) ||
			kd.Pending != src.Doc.Pending || kd.Timestamp != src.Doc.Timestamp {
			return false
		}
	}
	return true
}

// refreshKeywordCollections 用当前集合布局刷新索引内条目的集合归属（搬迁/启动加载后调用）。
func (d *Store) refreshKeywordCollections(idx *keywordIndex, sources []keywordIndexSource) {
	for _, src := range sources {
		if kd, ok := idx.docs[src.Doc.ID]; ok {
			kd.Collection = src.Collection
		}
	}
}

// ensureKeywordIndex 懒加载/重建倒排索引。索引不可用时只记录告警，不阻断检索降级。
func (d *Store) ensureKeywordIndex() {
	d.keywordMu.RLock()
	ready := d.keywordIndex != nil
	d.keywordMu.RUnlock()
	if ready {
		return
	}

	sources := d.keywordIndexSources()
	reused := false
	var idx *keywordIndex
	if d.baseDir != "" {
		if loaded, err := loadKeywordIndexFile(d.keywordIndexPath()); err == nil &&
			keywordIndexMatchesSources(loaded, sources) {
			d.refreshKeywordCollections(loaded, sources)
			idx = loaded
			reused = true
		}
	}
	if idx == nil {
		idx = buildKeywordIndexFromSources(sources)
	}

	d.keywordMu.Lock()
	if d.keywordIndex == nil {
		d.keywordIndex = idx
	} else {
		reused = true
		idx = d.keywordIndex
	}
	d.keywordMu.Unlock()

	if !reused {
		d.persistKeywordIndexSnapshot()
	}
}

// RebuildKeywordIndex 强制从已加载文档重建索引并原子落盘（不重新嵌入）。
// 供迁移/对账流程与运维显式重建使用；正常启动由 ensureKeywordIndex 幂等处理。
func (d *Store) RebuildKeywordIndex() error {
	sources := d.keywordIndexSources()
	idx := buildKeywordIndexFromSources(sources)
	d.keywordMu.Lock()
	d.keywordIndex = idx
	d.keywordMu.Unlock()
	if d.baseDir == "" {
		return nil
	}
	return writeKeywordIndexFile(d.keywordIndexPath(), keywordIndexFileFromIndex(idx))
}

func (d *Store) keywordIndexPath() string {
	return filepath.Join(d.baseDir, KeywordIndexFile)
}

// rebuildKeywordIndexIfReady 仅在索引已建立时重建（避免无谓的内存/磁盘开销）。
func (d *Store) rebuildKeywordIndexIfReady() {
	d.keywordMu.RLock()
	ready := d.keywordIndex != nil
	d.keywordMu.RUnlock()
	if !ready {
		return
	}
	if err := d.RebuildKeywordIndex(); err != nil {
		logger.Warn("FileManager", "关键词索引重建失败（沿用旧索引）: %v", err)
	}
}

// mutateKeywordIndex 在持锁状态下应用变更，随后生成快照并落盘（写新文件、原子替换）。
// 索引尚未建立时直接跳过：查询首次懒加载会从当前文档全量构建，不会丢变更。
func (d *Store) mutateKeywordIndex(fn func(*keywordIndex)) {
	d.keywordMu.Lock()
	idx := d.keywordIndex
	if idx == nil {
		d.keywordMu.Unlock()
		return
	}
	fn(idx)
	file := keywordIndexFileFromIndex(idx)
	d.keywordMu.Unlock()
	d.writeKeywordIndexSnapshot(file)
}

func (d *Store) persistKeywordIndexSnapshot() {
	d.keywordMu.RLock()
	idx := d.keywordIndex
	var file *keywordIndexFile
	if idx != nil {
		file = keywordIndexFileFromIndex(idx)
	}
	d.keywordMu.RUnlock()
	d.writeKeywordIndexSnapshot(file)
}

func (d *Store) writeKeywordIndexSnapshot(file *keywordIndexFile) {
	if d.baseDir == "" || file == nil {
		return
	}
	if err := writeKeywordIndexFile(d.keywordIndexPath(), file); err != nil {
		logger.Warn("FileManager", "关键词索引落盘失败（内存索引仍可用，下次启动将重建）: %v", err)
	}
}

// KeywordIndexStats 返回倒排索引规模与体积观测值。
// 体积估算口径：postings × 8B（序号+词频+切片开销）+ 词表 token 字节 + 条目元数据开销。
func (d *Store) KeywordIndexStats() KeywordIndexStats {
	d.ensureKeywordIndex()
	d.keywordMu.RLock()
	defer d.keywordMu.RUnlock()
	stats := KeywordIndexStats{}
	if d.keywordIndex == nil {
		return stats
	}
	stats.Docs = len(d.keywordIndex.docs)
	stats.Tokens = len(d.keywordIndex.tokens)
	stats.Postings = d.keywordIndex.postingCount()
	tokenBytes := 0
	for token := range d.keywordIndex.tokens {
		tokenBytes += len(token)
	}
	stats.EstimatedBytes = int64(stats.Postings*KeywordPostingBytesEstimate +
		tokenBytes + stats.Docs*96)
	if d.baseDir != "" {
		if info, err := os.Stat(d.keywordIndexPath()); err == nil {
			stats.FileBytes = info.Size()
		}
	}
	return stats
}
