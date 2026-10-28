package memory

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
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
		if len(doc.Embedding) == 0 {
			continue
		}
		if cosineSimilarity(vec, doc.Embedding) >= DedupThreshold {
			return doc.ID
		}
	}
	return ""
}

// findDuplicateByURL 按媒体实体 URL（内容哈希命名）判定是否已存在同一条记忆。
// 图片/音频的标签或描述可能相同，但实体不同；去重必须以实体为准。
func (c *Collection) findDuplicateByURL(url string) string {
	if url == "" {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, doc := range c.Documents {
		if doc.URL == url {
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
		ID:         doc.ID,
		Type:       docType,
		Role:       role,
		Content:    doc.Content,
		URL:        doc.URL,
		Base64:     doc.Base64,
		Collection: c.Name,
		Source:     doc.Source,
		Similarity: similarity,
		Timestamp:  doc.Timestamp,
	}
}

// routeCollection 内容归组：把内容嵌入向量与全部集合名嵌入向量比较，返回最相关的集合
//
// 不同集合可能锁定不同嵌入模型：按模型分组计算内容向量，保证向量空间一致
func (d *Store) routeCollection(ctx context.Context, content string) (*Collection, []float32, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil, fmt.Errorf("用于归组的内容为空")
	}

	collections := d.snapshotCollections()
	vectors := make(map[string][]float32)
	var best *Collection
	var bestVector []float32
	bestSim := float32(-2)

	model := currentEmbeddingModel()
	for _, c := range collections {
		c.mu.RLock()
		name := c.Name
		nameEmbedding := append([]float32(nil), c.NameEmbedding...)
		c.mu.RUnlock()
		if len(nameEmbedding) == 0 {
			continue
		}

		vec, ok := vectors[model]
		if !ok {
			embedded, err := d.embedText(ctx, model, content)
			if err != nil {
				logger.Warn("FileManager", "集合 [%s] 内容归组嵌入失败（模型 %s）: %v", name, model, err)
				continue
			}
			vectors[model] = embedded
			vec = embedded
		}
		if len(vec) != len(nameEmbedding) {
			continue
		}

		sim := cosineSimilarity(vec, nameEmbedding)
		if sim > bestSim || (sim == bestSim && (best == nil || name < best.Name)) {
			best = c
			bestVector = vec
			bestSim = sim
		}
	}

	if best == nil {
		// 兜底：集合名向量尚未生成（如嵌入服务刚启动）时，至少把内容写进一个可用分组，
		// 避免直接丢记忆；下次启动补齐集合名向量后，新内容仍会正常归组
		for _, c := range collections {
			vec, err := d.embedText(ctx, currentEmbeddingModel(), content)
			if err != nil {
				continue
			}
			logger.Warn("FileManager", "集合名向量缺失，内容暂存到分组 [%s]（待下次启动补齐后恢复归组）", c.Name)
			return c, vec, nil
		}
		return nil, nil, fmt.Errorf("没有可用的记忆分组（集合名嵌入向量为空）")
	}
	return best, bestVector, nil
}

// appendDocument 追加文档并立即持久化
func (c *Collection) appendDocument(doc Document) error {
	c.mu.Lock()
	c.Documents = append(c.Documents, doc)
	c.mu.Unlock()
	if err := c.saveDocumentsToFile(); err != nil {
		return fmt.Errorf("保存文档失败: %w", err)
	}
	return nil
}
