package memory

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// CollectionCount 返回所有集合的数量
func (d *Store) CollectionCount() int {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	return len(d.collections)
}

// Documents 获取集合中文档的分页列表
func (d *Store) Documents(collectionName string, offset, limit int) ([]DocumentEntry, int) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil, 0
	}

	c.reloadIfChanged()

	c.mu.RLock()
	defer c.mu.RUnlock()

	total := len(c.Documents)
	if total == 0 || offset >= total {
		return nil, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	entries := make([]DocumentEntry, end-offset)
	for i := offset; i < end; i++ {
		doc := c.Documents[i]
		docType := documentType(doc)
		role := doc.Role
		if role == "" && docType != TypeText {
			role = docType
		}
		entries[i-offset] = DocumentEntry{
			ID:         doc.ID,
			Type:       docType,
			Role:       role,
			Content:    doc.Content,
			URL:        doc.URL,
			Base64:     doc.Base64,
			Collection: c.Name,
			Source:     doc.Source,
			Timestamp:  doc.Timestamp,
		}
	}
	return entries, total
}

// DocCount 返回指定集合的文档总数
func (d *Store) DocCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	return c.docCount()
}

// Reload 从磁盘重新加载指定分组。
// 供前端管理工具直接改写 documents_NNNN.json 后刷新进程内缓存（不重建向量）。
func (d *Store) Reload(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}
	if err := c.loadDocumentsFromFile(); err != nil {
		return fmt.Errorf("重新加载分组失败: %w", err)
	}
	c.updateLastModTime()
	return nil
}

// HasSyncMismatch 检查集合中是否存在缺失内容向量的文档
func (d *Store) HasSyncMismatch(collectionName string) bool {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return false
	}

	c.reloadIfChanged()

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, doc := range c.Documents {
		if len(doc.Embedding) == 0 {
			return true // 存在缺失内容向量的文档
		}
	}
	return false
}

// DeleteCollection 删除整个集合及其所有数据
func (d *Store) DeleteCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	c.mu.RLock()
	docs := append([]Document(nil), c.Documents...)
	c.mu.RUnlock()

	d.collectionsMu.Lock()
	delete(d.collections, collectionName)
	d.collectionsMu.Unlock()

	// 先移出集合注册表，再清理媒体：引用检查只会看到其它集合，不会误判本集合仍在使用
	d.removeMemoryMediaForDocs(collectionName, docs)

	if err := os.RemoveAll(c.collDir); err != nil {
		return fmt.Errorf("删除集合目录失败: %w", err)
	}
	return nil
}

// ClearCollection 清空集合所有数据，保留 metadata.json
func (d *Store) ClearCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	c.mu.RLock()
	docs := append([]Document(nil), c.Documents...)
	c.mu.RUnlock()

	c.mu.Lock()
	c.Documents = make([]Document, 0)
	c.mu.Unlock()

	// 删除所有分块文件（文档 + 嵌入向量）
	files, _ := filepath.Glob(filepath.Join(c.collDir, "documents_*.json"))
	for _, f := range files {
		os.Remove(f)
	}
	embFiles, _ := filepath.Glob(filepath.Join(c.collDir, "embeddings_*.json"))
	for _, f := range embFiles {
		os.Remove(f)
	}

	c.mu.Lock()
	c.documentsChunkCount = 0
	c.embeddingsChunkCount = 0
	c.mu.Unlock()

	if err := c.saveCollectionMeta(); err != nil {
		return err
	}

	// 文档已清空，再清理不再被任何条目引用的媒体实体
	d.removeMemoryMediaForDocs(collectionName, docs)
	return nil
}

// Collections 返回所有集合名
func (d *Store) Collections() []string {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()

	names := make([]string, 0, len(d.collections))
	for name := range d.collections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CollectionInfo 返回指定集合的详细元数据信息
func (d *Store) CollectionInfo(collectionName string) map[string]interface{} {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"name":                     c.Name,
		"embedding_dimension":      c.Dimension,
		"name_embedding_model":     c.NameEmbeddingModel,
		"name_embedding_dimension": len(c.NameEmbedding),
		"has_name_embedding":       len(c.NameEmbedding) > 0,
		"version":                  CurrentVersion,
		"document_count":           len(c.Documents),
		"documents_chunk_count":    c.documentsChunkCount,
	}
}

// Rebuild 重建集合的文档内容嵌入向量：
// 文本记忆以正文重新嵌入；图片记忆重新调用多模态生成标签并以标签文本重新嵌入。
// 当嵌入模型变更时调用。
func (d *Store) Rebuild(ctx context.Context, collectionName, modelName string, progress func(int, int)) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	c.mu.RLock()
	docs := make([]Document, len(c.Documents))
	copy(docs, c.Documents)
	c.mu.RUnlock()

	// 目标模型（modelName 为空时沿用当前配置模型）
	effectiveModel := currentEmbeddingModel()
	if modelName != "" {
		effectiveModel = modelName
	}

	for i, doc := range docs {
		if progress != nil {
			progress(i+1, len(docs))
		}

		var contentVec []float32
		if documentType(doc) == TypeImage {
			// 图片记忆：重新多模态生成标签并以标签文本嵌入（URL 记忆先读回文件）
			imageSource := doc.Base64
			if imageSource == "" {
				imageSource = memoryImageDataURI(doc.URL)
			}
			d.llmMu.Lock()
			tags, terr := d.generateTags(ctx, imageSource, true, RecognitionAuto, "")
			d.llmMu.Unlock()
			if terr != nil {
				logger.Warn("FileManager", "重建图片标签失败 [%s]: %v", doc.ID, terr)
				continue
			}
			contentVec, err = d.embedText(ctx, effectiveModel, formatTagsText(tags))
		} else {
			// 文本 / 音频记忆：以正文（音频为描述文本）重新嵌入
			contentVec, err = d.embedText(ctx, effectiveModel, doc.Content)
		}
		if err != nil {
			logger.Warn("FileManager", "嵌入内容失败 [%s]: %v", doc.ID, err)
			continue
		}

		c.mu.Lock()
		c.Dimension = len(contentVec)
		for j := range c.Documents {
			if c.Documents[j].ID == doc.ID {
				c.Documents[j].Embedding = contentVec
				break
			}
		}
		c.mu.Unlock()
	}

	if err := c.saveDocumentsToFile(); err != nil {
		return fmt.Errorf("保存文档失败: %w", err)
	}
	// 模型变更后集合名向量也必须同步重建，否则内容归组仍在旧向量空间里比较
	c.mu.RLock()
	nameStale := len(c.NameEmbedding) == 0 || c.NameEmbeddingModel != effectiveModel
	c.mu.RUnlock()
	if nameStale {
		c.mu.Lock()
		c.NameEmbedding = nil
		c.NameEmbeddingModel = ""
		c.mu.Unlock()
		if err := d.ensureCollectionNameEmbedding(ctx, c); err != nil {
			logger.Warn("FileManager", "重建集合 [%s] 名称嵌入失败: %v", c.Name, err)
		}
	}
	if err := c.saveCollectionMeta(); err != nil {
		return fmt.Errorf("保存元数据失败: %w", err)
	}

	return nil
}

// memoryImageDataURI 把记忆图 URL 读回 data URI（重建标签向量使用）
func memoryImageDataURI(url string) string {
	rel := strings.TrimPrefix(strings.SplitN(url, "?", 2)[0], "/file/read/")
	if rel == "" || !strings.HasPrefix(rel, "multimedia/") {
		return url
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		return url
	}
	mimeType := "image/png"
	if detected, ok := persistence.PreviewMIME(filepath.Ext(abs)); ok {
		mimeType = detected
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}
