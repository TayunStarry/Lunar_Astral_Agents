package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// validateCollectionName 验证集合名格式
func (d *Store) validateCollectionName(name string) error {
	if name == "" {
		return fmt.Errorf("集合名不能为空")
	}
	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return fmt.Errorf("集合名包含非法字符: %s", name)
	}
	return nil
}

// isCollectionValid 检查集合是否合法且已初始化
func (d *Store) isCollectionValid(name string) error {
	if err := d.validateCollectionName(name); err != nil {
		return err
	}
	if !d.initialized {
		return fmt.Errorf("记忆库实例未初始化，请先调用 EnsureReady")
	}
	return nil
}

// InitCollection 初始化或打开一个分组集合。
//
// 集合是内容自动归入的语义分组：创建时立即用集合名调用嵌入模型，生成集合名向量。
func (d *Store) InitCollection(ctx context.Context, name string) error {
	if err := d.isCollectionValid(name); err != nil {
		return err
	}

	// 已在缓存：补齐磁盘目录与集合名向量即可
	d.collectionsMu.RLock()
	existing, exists := d.collections[name]
	d.collectionsMu.RUnlock()
	if exists {
		if err := os.MkdirAll(filepath.Join(d.baseDir, name), 0755); err != nil {
			return fmt.Errorf("确保集合目录存在失败: %w", err)
		}
		return d.ensureCollectionNameEmbedding(ctx, existing)
	}

	collDir := filepath.Join(d.baseDir, name)
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return fmt.Errorf("创建集合目录失败: %w", err)
	}

	metaPath := filepath.Join(collDir, "metadata.json")
	modelName := currentEmbeddingModel()

	// 已存在元数据：按现有数据加载（模型保持集合自身锁定的值，不再因配置变更清空用户记忆）
	recreate := false
	if _, err := os.Stat(metaPath); err == nil {
		var meta collectionMeta
		if err := readJSONFile(metaPath, &meta); err != nil {
			return fmt.Errorf("读取元数据失败: %w", err)
		}
		if meta.Version < CurrentVersion || meta.EmbeddingDimension == 0 {
			logger.Warn("FileManager", "集合 %s 元数据不满足新格式 (version=%d, dim=%d)，清空重建",
				name, meta.Version, meta.EmbeddingDimension)
			clearCollectionDir(collDir)
			recreate = true
		}
	}

	if !recreate {
		if _, err := os.Stat(metaPath); err == nil {
			var meta collectionMeta
			if err := readJSONFile(metaPath, &meta); err != nil {
				return fmt.Errorf("读取元数据失败: %w", err)
			}
			c := &Collection{
				Name:               name,
				Dimension:          meta.EmbeddingDimension,
				NameEmbedding:      meta.NameEmbedding,
				NameEmbeddingModel: meta.NameEmbeddingModel,
				collDir:            collDir,
				metaPath:           metaPath,
			}
			if err := c.loadDocumentsFromFile(); err != nil {
				return fmt.Errorf("加载文档失败: %w", err)
			}
			c.updateLastModTime()

			d.collectionsMu.Lock()
			d.collections[name] = c
			d.collectionsMu.Unlock()

			return d.ensureCollectionNameEmbedding(ctx, c)
		}
	}

	// 全新创建：集合名向量同时决定向量维度，不再额外发探针嵌入请求
	nameVec, err := d.embedText(ctx, modelName, name)
	if err != nil {
		return fmt.Errorf("集合名嵌入失败: %w", err)
	}

	c := &Collection{
		Name:               name,
		Dimension:          len(nameVec),
		NameEmbedding:      nameVec,
		NameEmbeddingModel: modelName,
		Documents:          make([]Document, 0),
		collDir:            collDir,
		metaPath:           metaPath,
	}

	if err := c.saveCollectionMeta(); err != nil {
		return fmt.Errorf("保存元数据失败: %w", err)
	}
	c.updateLastModTime()

	d.collectionsMu.Lock()
	d.collections[name] = c
	d.collectionsMu.Unlock()

	return nil
}

// clearCollectionDir 清空集合目录内旧数据文件（不做旧版数据迁移，直接废弃）
func clearCollectionDir(collDir string) {
	patterns := []string{"documents_*.json", "images_*.json", "tags_*.json", "contents_*.json", "embeddings_*.json", "base64_*.json", "documents.json", "metadata.json"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(collDir, pattern))
		for _, f := range files {
			os.Remove(f)
		}
	}
}

// getCollection 获取集合实例（线程安全）
func (d *Store) getCollection(name string) (*Collection, error) {
	d.collectionsMu.RLock()
	c, ok := d.collections[name]
	d.collectionsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("集合 '%s' 不存在，请先初始化", name)
	}
	return c, nil
}

// snapshotCollections 返回全部集合的快照（用于全局检索与内容归组）
func (d *Store) snapshotCollections() []*Collection {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	collections := make([]*Collection, 0, len(d.collections))
	for _, c := range d.collections {
		collections = append(collections, c)
	}
	// 固定顺序：归组在相似度并列时按集合名兜底，保证结果可复现
	sort.Slice(collections, func(i, j int) bool { return collections[i].Name < collections[j].Name })
	return collections
}

// loadAllCollections 从磁盘加载所有集合（启动时一次载入全部）
func (d *Store) loadAllCollections() {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		logger.Error("FileManager", "读取记忆库目录失败: %v", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		metaPath := filepath.Join(d.baseDir, name, "metadata.json")
		var meta collectionMeta
		if err := readJSONFile(metaPath, &meta); err != nil {
			continue
		}

		// v5 版本检查
		if meta.Version < CurrentVersion {
			logger.Warn("FileManager", "集合 %s 版本过低 (%d < %d)，跳过加载", name, meta.Version, CurrentVersion)
			continue
		}
		if meta.EmbeddingDimension == 0 {
			logger.Warn("FileManager", "集合 %s 元数据不完整，跳过加载", name)
			continue
		}

		c := &Collection{
			Name:               name,
			Dimension:          meta.EmbeddingDimension,
			NameEmbedding:      meta.NameEmbedding,
			NameEmbeddingModel: meta.NameEmbeddingModel,
			collDir:            filepath.Join(d.baseDir, name),
			metaPath:           metaPath,
		}

		if err := c.loadDocumentsFromFile(); err != nil {
			logger.Error("FileManager", "加载集合 %s 文档失败: %v", name, err)
			continue
		}
		c.updateLastModTime()

		d.collectionsMu.Lock()
		d.collections[name] = c
		d.collectionsMu.Unlock()

		logger.Info("FileManager", "已加载集合 %s (%d 文档, 名称向量=%v)", name, len(c.Documents), len(c.NameEmbedding) > 0)
	}
}

// =============================================================================
// 文件路径辅助函数
// =============================================================================

// documentsFilePath 返回文档分块文件路径
func (c *Collection) documentsFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("documents_%04d.json", chunkNum))
}

// embeddingsFilePath 返回嵌入向量分块文件路径
func (c *Collection) embeddingsFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("embeddings_%04d.json", chunkNum))
}

// =============================================================================
// 持久化 — 元数据、文档
// =============================================================================

// saveCollectionMeta 保存集合元数据到 metadata.json
func (c *Collection) saveCollectionMeta() error {
	c.mu.RLock()
	meta := collectionMeta{
		EmbeddingDimension:   c.Dimension,
		NameEmbedding:        append([]float32(nil), c.NameEmbedding...),
		NameEmbeddingModel:   c.NameEmbeddingModel,
		Version:              CurrentVersion,
		DocumentsChunkCount:  c.documentsChunkCount,
		EmbeddingsChunkCount: c.embeddingsChunkCount,
	}
	c.mu.RUnlock()
	return atomicWriteJSON(c.metaPath, meta)
}

// updateLastModTime 更新最后修改时间戳
func (c *Collection) updateLastModTime() {
	if info, err := os.Stat(c.metaPath); err == nil {
		c.lastFileModTime = info.ModTime()
	}
}

// loadDocumentsFromFile 从分块文件加载文档与嵌入向量到内存
// documents_NNNN.json 存文档本体（不含向量），embeddings_NNNN.json 存向量并以 ID 关联
func (c *Collection) loadDocumentsFromFile() error {
	var allDocs []Document

	for i := 1; ; i++ {
		path := c.documentsFilePath(i)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		var docs []Document
		if err := readJSONFile(path, &docs); err != nil {
			return fmt.Errorf("读取 %s 失败: %w", path, err)
		}
		allDocs = append(allDocs, docs...)
		c.documentsChunkCount = i
	}

	// 加载嵌入向量切片并按 ID 挂回文档
	embMap := make(map[string][]float32, len(allDocs))
	for i := 1; ; i++ {
		path := c.embeddingsFilePath(i)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		var records []embeddingRecord
		if err := readJSONFile(path, &records); err != nil {
			return fmt.Errorf("读取 %s 失败: %w", path, err)
		}
		for _, rec := range records {
			embMap[rec.ID] = rec.Embedding
		}
		c.embeddingsChunkCount = i
	}

	for j := range allDocs {
		if vec, ok := embMap[allDocs[j].ID]; ok {
			allDocs[j].Embedding = vec
		}
		// 旧数据没有 type 字段：按内容推断并回写，保证后续类型化查询可用
		if allDocs[j].Type == "" {
			allDocs[j].Type = documentType(allDocs[j])
		}
	}

	c.mu.Lock()
	c.Documents = allDocs
	c.mu.Unlock()

	return nil
}

// saveDocumentsToFile 将文档与嵌入向量保存到分块文件（100 条一块）
// 文档写入 documents_NNNN.json（Embedding json:"-" 不落盘）
// 嵌入向量写入 embeddings_NNNN.json，两份切片按相同索引一一对应
func (c *Collection) saveDocumentsToFile() error {
	c.mu.RLock()
	docs := make([]Document, len(c.Documents))
	copy(docs, c.Documents)
	c.mu.RUnlock()

	chunkSize := DocumentsChunkSize
	oldDocCount := c.documentsChunkCount
	oldEmbCount := c.embeddingsChunkCount

	total := len(docs)
	newChunkCount := (total + chunkSize - 1) / chunkSize
	if total == 0 {
		newChunkCount = 0
	}

	// 写入各分块（文档 + 嵌入向量）
	for i := 0; i < newChunkCount; i++ {
		chunkNum := i + 1
		start := i * chunkSize
		end := start + chunkSize
		if end > total {
			end = total
		}

		docChunk := docs[start:end]
		if err := atomicWriteJSON(c.documentsFilePath(chunkNum), docChunk); err != nil {
			return err
		}

		embChunk := make([]embeddingRecord, end-start)
		for j := range embChunk {
			d := docChunk[j]
			embChunk[j] = embeddingRecord{ID: d.ID, Embedding: d.Embedding}
		}
		if err := atomicWriteJSON(c.embeddingsFilePath(chunkNum), embChunk); err != nil {
			return err
		}
	}

	// 清理多余的分块文件
	for i := newChunkCount + 1; i <= oldDocCount; i++ {
		os.Remove(c.documentsFilePath(i))
	}
	for i := newChunkCount + 1; i <= oldEmbCount; i++ {
		os.Remove(c.embeddingsFilePath(i))
	}

	c.mu.Lock()
	c.documentsChunkCount = newChunkCount
	c.embeddingsChunkCount = newChunkCount
	c.mu.Unlock()

	return c.saveCollectionMeta()
}

// reloadIfChanged 检查 metadata.json 是否被外部进程修改，若是则重新加载
func (c *Collection) reloadIfChanged() {
	if info, err := os.Stat(c.metaPath); err != nil {
		return
	} else if !info.ModTime().Equal(c.lastFileModTime) {
		c.loadDocumentsFromFile()
		c.updateLastModTime()
	}
}
