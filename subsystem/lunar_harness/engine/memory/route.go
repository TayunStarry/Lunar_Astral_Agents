package memory

import (
	"fmt"
	"strings"
)

// docCount 返回集合中文档总数（线程安全）
func (c *Collection) docCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Documents)
}

// formatTagsText 将一组标签按 "[标签1],[标签2]" 格式拼接为用于嵌入的文本
func formatTagsText(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "[" + strings.Join(tags, "],[") + "]"
}

// findDuplicateByEmbedding 基于内容嵌入向量判定是否与既有记忆重复。
// 余弦相似度 ≥ DedupThreshold 视为重复，返回已存在文档 ID；否则返回空字符串。
func (c *Collection) findDuplicateByEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, doc := range c.Documents {
		// 只与文本记忆比较高相似度：媒体记忆的向量来自标签/描述，不应被文本写入复用
		if documentType(doc) != TypeText || len(doc.Embedding) == 0 {
			continue
		}
		if cosineSimilarity(vec, doc.Embedding) >= DedupThreshold {
			return doc.ID
		}
	}
	return ""
}

// documentType 返回文档的记忆类型；旧数据没有 type 字段时按内容推断
func documentType(doc Document) string {
	if doc.Type != "" {
		return doc.Type
	}
	if doc.URL != "" {
		lower := strings.ToLower(doc.URL)
		if strings.Contains(lower, "/audios/") || strings.Contains(lower, ".mp3") ||
			strings.Contains(lower, ".wav") || strings.Contains(lower, ".ogg") ||
			strings.Contains(lower, ".m4a") || strings.Contains(lower, ".flac") {
			return TypeAudio
		}
		return TypeImage
	}
	if doc.Base64 != "" {
		return TypeImage
	}
	return TypeText
}

// NormalizeType 校验并归一化记忆类型；空值默认按文本记忆处理
func NormalizeType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", TypeText:
		return TypeText, nil
	case TypeImage:
		return TypeImage, nil
	case TypeAudio:
		return TypeAudio, nil
	default:
		return "", fmt.Errorf("未知的记忆类型: %q（支持 text/image/audio）", value)
	}
}

// resultOf 把文档转换为查询/列举结果（附带所属集合与类型）
func resultOf(c *Collection, doc *Document, similarity float32) Entry {
	docType := documentType(*doc)
	role := doc.Role
	if role == "" && docType != TypeText {
		role = docType
	}
	return Entry{
		ID:             doc.ID,
		Type:           docType,
		Role:           role,
		Content:        doc.Content,
		URL:            doc.URL,
		Base64:         doc.Base64,
		Collection:     c.Name,
		Source:         doc.Source,
		Similarity:     similarity,
		Timestamp:      doc.Timestamp,
		Pending:        doc.Pending,
		PendingPartner: doc.PendingPartner,
	}
}

// contentVectorOf 返回用于判重/聚类/整理校验的正文向量。
// 带 tags 写入时 Embedding 是标签向量，正文向量单独保存在 ContentEmbedding；
// 无标签写入与旧数据没有 ContentEmbedding，此时 Embedding 本身就是正文向量。
func contentVectorOf(doc *Document) []float32 {
	if len(doc.ContentEmbedding) > 0 {
		return doc.ContentEmbedding
	}
	return doc.Embedding
}

// findDuplicateText 跨全部簇查找与正文向量高度相似的文本记忆。
// 余弦相似度 ≥ DedupThreshold 视为重复，返回最相似的条目 ID；否则返回空。
func (d *Store) findDuplicateText(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	bestID := ""
	bestSim := float32(0)
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		for i := range c.Documents {
			doc := &c.Documents[i]
			if documentType(*doc) != TypeText {
				continue
			}
			docVec := contentVectorOf(doc)
			if len(docVec) != len(vec) {
				continue
			}
			sim := cosineSimilarity(vec, docVec)
			if sim >= DedupThreshold && sim > bestSim {
				bestSim = sim
				bestID = doc.ID
			}
		}
		c.mu.RUnlock()
	}
	return bestID
}

// findDuplicateMediaByURL 跨全部簇按媒体实体内容判重。
// 图片/音频落在 <子树>/memory/<条目ID>/<内容哈希>，完整 URL 每次写入都不同，
// 因此比较「子树类型 + 内容哈希文件名」，而不是完整 URL。
func (d *Store) findDuplicateMediaByURL(url string) string {
	key := mediaContentKey(url)
	if key == "" {
		return ""
	}
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		for i := range c.Documents {
			doc := &c.Documents[i]
			if mediaContentKey(doc.URL) != key {
				continue
			}
			id := doc.ID
			c.mu.RUnlock()
			return id
		}
		c.mu.RUnlock()
	}
	return ""
}

// clusterStorage 返回指定簇的物理存储单元；clusterID 为空时返回总库。
func (d *Store) clusterStorage(clusterID string) (*Collection, error) {
	if clusterID == "" {
		return d.storeCollection()
	}
	return d.collectionForCluster(clusterID, clusterID == StoreCollectionID)
}

// appendDocument 追加文档并立即持久化
func (c *Collection) appendDocument(doc Document) error {
	c.mu.Lock()
	c.Documents = append(c.Documents, doc)
	if c.positions == nil {
		c.rebuildPositionsLocked()
	} else if doc.ID != "" {
		c.positions[doc.ID] = len(c.Documents) - 1
	}
	c.mu.Unlock()
	if err := c.saveDocumentsToFile(); err != nil {
		return fmt.Errorf("保存文档失败: %w", err)
	}
	return nil
}
