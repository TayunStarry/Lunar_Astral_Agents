package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// AddText 添加文本消息到记忆库（内容自动归入最相关的分组集合）
func (d *Store) AddText(ctx context.Context, role, content string) (string, error) {
	return d.AddTextWithTags(ctx, role, content, nil, "")
}

// AddTextWithTags 添加文本消息并携带预设标签（写入来源可标记，如 action）。
//
// 标签文本只用于嵌入与归组；实际对话正文仍存入 content 字段。
func (d *Store) AddTextWithTags(ctx context.Context, role, content string, tags []string, source string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("消息内容不能为空")
	}

	// 1. 确定用于嵌入与归组的文本：有预设标签用标签拼接文本，否则用正文全文
	embeddingText := content
	if len(tags) > 0 {
		embeddingText = formatTagsText(tags)
	}

	// 2. 计算内容向量（归簇与判重都用它）
	contentVec, err := d.embedText(ctx, currentEmbeddingModel(), embeddingText)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}

	// 3. 高度相似条目不再丢弃新措辞：新条目照常写入并标记为待处理，
	//    由整理流程聚类后合并。队列超过写入上限时仍照常写入，只是不再入队，
	//    保证任何情况下都不会先丢数据。
	// 判重：跨总库 + 全部正式簇扫描（DedupThreshold=0.98）。
	duplicateID := d.findDuplicateText(contentVec)
	pending := duplicateID != ""
	if pending && d.pendingCount() >= PendingMaxEntries {
		logger.Warn("FileManager", "待处理队列已达上限 %d 条，新记忆暂停入队（仍照常写入）: 相似 ID=%s",
			PendingMaxEntries, duplicateID)
		pending = false
		duplicateID = ""
	}

	// 4. 新条目统一写入总库；正式簇只在迁移/重建时从总库中升级产生
	id := generateUUID()
	if err := d.assignToStore(id, contentVec); err != nil {
		return "", fmt.Errorf("写入总库失败: %w", err)
	}
	c, err := d.storeCollection()
	if err != nil {
		return "", err
	}

	doc := Document{
		ID:        id,
		Type:      TypeText,
		Role:      role,
		Content:   content,
		Source:    source,
		Embedding: contentVec,
		Timestamp: time.Now().Unix(),
	}
	if pending {
		doc.Pending = true
		doc.PendingPartner = duplicateID
	}
	if err := c.appendDocument(doc); err != nil {
		d.detachCluster(id)
		return "", err
	}
	d.keywordIndexAdd(c, doc)
	if pending {
		logger.Info("FileManager", "总库命中高度相似记忆 %s，新措辞已写入待处理队列 (ID=%s)",
			duplicateID, id)
	}
	return id, nil
}

// AddImage 添加图片到记忆库（自动归组）。
//
// 调用多模态模型生成画面标签，以标签文本计算内容向量并选择分组；
// 图片实体落到 multimedia/memory/<集合名>/，记忆库只保存 URL。
func (d *Store) AddImage(ctx context.Context, base64Image string, orientation string, custom string) (string, error) {
	if strings.TrimSpace(base64Image) == "" {
		return "", fmt.Errorf("图片数据不能为空")
	}

	// 1. 感知者式标准化：得到入库 URI（静态图缩放/转码；动态图保留动画）与视频理解引用
	storeURI, mediaRefs, animated := standardizeMemoryImage(base64Image)

	// 2. 多模态标签化理解（单线程 + 同步阻塞）：动态图走统一观看管线抽帧，静态图走单图
	d.llmMu.Lock()
	var tags []string
	var err error
	if animated && len(mediaRefs) > 0 {
		tags, err = d.generateTagsFromMedia(ctx, mediaRefs, orientation, custom)
		if err != nil {
			// 视频链路失败（如非 llama-server 后端）：回退单图理解再试一轮。
			// 优先用首帧而不是整张动图：多数多模态模型只可靠解码静态帧。
			logger.Warn("Persistence", "动态图帧序列理解失败（%v），回退单图理解", err)
			fallback := storeURI
			if len(mediaRefs) > 0 {
				fallback = mediaRefs[0]
			}
			tags, err = d.generateTags(ctx, fallback, true, orientation, custom)
		}
	} else {
		tags, err = d.generateTags(ctx, storeURI, true, orientation, custom)
	}
	d.llmMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("标签生成失败: %w", err)
	}

	// 3. 以标签文本计算内容向量
	tagsText := formatTagsText(tags)
	if tagsText == "" {
		tagsText = "图片"
	}
	contentVec, err := d.embedText(ctx, currentEmbeddingModel(), tagsText)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}

	id := generateUUID()
	// 4. 图片实体落到 multimedia/memory/<条目ID>/，记忆库只保存 URL（不再内联 base64）
	url, err := saveMemoryImageFile(id, storeURI)
	if err != nil {
		return "", fmt.Errorf("图片落盘失败: %w", err)
	}

	// 5. 同一张图片（内容哈希相同）只保留一条记忆；不同图片即使标签相同也分别入库
	if existingID := d.findDuplicateMediaByURL(url); existingID != "" {
		discardSavedMediaFile(url)
		logger.Info("FileManager", "已存在相同图片记忆 (ID=%s)，跳过重复写入", existingID)
		return existingID, nil
	}

	if err := d.assignToStore(id, contentVec); err != nil {
		return "", fmt.Errorf("写入总库失败: %w", err)
	}
	c, err := d.storeCollection()
	if err != nil {
		return "", err
	}
	doc := Document{
		ID:        id,
		Type:      TypeImage,
		URL:       url,
		Embedding: contentVec,
		Timestamp: time.Now().Unix(),
	}
	if err := c.appendDocument(doc); err != nil {
		d.detachCluster(id)
		return "", err
	}
	d.keywordIndexAdd(c, doc)
	return id, nil
}

// AddAudio 添加音频记忆（自动归组）。
//
// 音频本身不做内容理解，使用调用方给出的描述文本计算嵌入与归组；描述为空时按“音频”处理。
func (d *Store) AddAudio(ctx context.Context, base64Audio string, description string) (string, error) {
	if strings.TrimSpace(base64Audio) == "" {
		return "", fmt.Errorf("音频数据不能为空")
	}
	embeddingText := strings.TrimSpace(description)
	if embeddingText == "" {
		embeddingText = "音频"
	}
	contentVec, err := d.embedText(ctx, currentEmbeddingModel(), embeddingText)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}
	id := generateUUID()
	url, err := saveMemoryAudioFile(id, base64Audio)
	if err != nil {
		return "", fmt.Errorf("音频落盘失败: %w", err)
	}
	// 同一份音频（内容哈希相同）只保留一条记忆
	if existingID := d.findDuplicateMediaByURL(url); existingID != "" {
		discardSavedMediaFile(url)
		return existingID, nil
	}
	if err := d.assignToStore(id, contentVec); err != nil {
		return "", fmt.Errorf("写入总库失败: %w", err)
	}
	c, err := d.storeCollection()
	if err != nil {
		return "", err
	}
	doc := Document{
		ID:        id,
		Type:      TypeAudio,
		Content:   strings.TrimSpace(description),
		URL:       url,
		Embedding: contentVec,
		Timestamp: time.Now().Unix(),
	}
	if err := c.appendDocument(doc); err != nil {
		d.detachCluster(id)
		return "", err
	}
	d.keywordIndexAdd(c, doc)
	return id, nil
}

// Get 按 ID 返回一条记忆（跨全部集合查找）
func (d *Store) Get(id string) (*Entry, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("记忆 ID 不能为空")
	}
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		for i := range c.Documents {
			if c.Documents[i].ID != id {
				continue
			}
			result := resultOf(c, &c.Documents[i], 0)
			c.mu.RUnlock()
			return &result, nil
		}
		c.mu.RUnlock()
	}
	return nil, ErrNotFound
}

// List 逐一输出记忆条目（跨全部集合）。
//
// mediaType 为空表示不限类型；offset/limit 用于分页，limit <= 0 表示返回全部。
// 返回按入库时间倒序的条目与过滤后的总数。
func (d *Store) List(mediaType string, offset, limit int) ([]Entry, int) {
	return d.list(mediaType, offset, limit)
}

// ListWithError 与 List 相同，但把「记忆库不可用」表达成错误。
//
// 未初始化或启动加载失败时返回错误，调用方据此区分「读不到数据」与「确实没有数据」。
func (d *Store) ListWithError(mediaType string, offset, limit int) ([]Entry, int, error) {
	if !d.initialized {
		return nil, 0, fmt.Errorf("记忆库未就绪")
	}
	if err := d.loadError(); err != nil {
		return nil, 0, fmt.Errorf("记忆库加载失败: %w", err)
	}
	entries, total := d.list(mediaType, offset, limit)
	return entries, total, nil
}

// list 是分页列举的实际实现，供 List / ListWithError 共用。
func (d *Store) list(mediaType string, offset, limit int) ([]Entry, int) {
	filter := strings.ToLower(strings.TrimSpace(mediaType))
	if filter != "" {
		if normalized, err := NormalizeType(filter); err == nil {
			filter = normalized
		} else {
			return nil, 0
		}
	}
	if offset < 0 {
		offset = 0
	}

	var entries []Entry
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		for i := range c.Documents {
			doc := &c.Documents[i]
			if filter != "" && documentType(*doc) != filter {
				continue
			}
			entries = append(entries, resultOf(c, doc, 0))
		}
		c.mu.RUnlock()
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	total := len(entries)
	if offset >= total {
		return nil, total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return entries[offset:end], total
}

// Count 返回指定类型（空=全部）的记忆条目总数
func (d *Store) Count(mediaType string) int {
	_, total := d.List(mediaType, 0, 1)
	return total
}

// PendingCount 返回全部集合中待处理（待合并）条目的数量。
// 只看进程内已加载数据，不做跨进程重载，避免每次写入都扫盘。
func (d *Store) PendingCount() int {
	return d.pendingCount()
}

func (d *Store) pendingCount() int {
	total := 0
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		for _, doc := range c.Documents {
			if doc.Pending {
				total++
			}
		}
		c.mu.RUnlock()
	}
	return total
}

// ClearPending 清除指定条目的待处理标记（合并落库后调用）。
// 条目不存在或本来就不是待处理时直接返回 nil。
func (d *Store) ClearPending(ctx context.Context, id string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("记忆 ID 不能为空")
	}
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.Lock()
		found := false
		changed := false
		for i := range c.Documents {
			if c.Documents[i].ID != id {
				continue
			}
			found = true
			if c.Documents[i].Pending || c.Documents[i].PendingPartner != "" {
				c.Documents[i].Pending = false
				c.Documents[i].PendingPartner = ""
				changed = true
			}
			break
		}
		c.mu.Unlock()
		if !found {
			continue
		}
		if !changed {
			return nil
		}
		if err := c.saveDocumentsToFile(); err != nil {
			return fmt.Errorf("保存待处理标记失败: %w", err)
		}
		// 待处理条目仍建索引（仅查询时过滤），但需同步标记避免启动对账触发全量重建
		d.keywordIndexSetPending(id, false)
		c.updateLastModTime()
		return nil
	}
	return ErrNotFound
}

// ListBySource 按写入来源列举条目（按入库时间倒序；limit<=0 表示不限）。
//
// 来源只是统一记忆库里的写入标记，用来把某个智能体写入的片段取回来做上下文，
// 不构成独立记忆库，也不复制任何存储。
func (d *Store) ListBySource(mediaType, source string, limit int) []Entry {
	want := strings.TrimSpace(source)
	if want == "" {
		return nil
	}
	capacity := limit
	if capacity < 0 {
		capacity = 0
	}
	const pageSize = 200
	out := make([]Entry, 0, capacity)
	for offset := 0; ; offset += pageSize {
		entries, total := d.List(mediaType, offset, pageSize)
		for _, entry := range entries {
			if entry.Source != want {
				continue
			}
			out = append(out, entry)
			if limit > 0 && len(out) >= limit {
				return out
			}
		}
		if len(entries) == 0 || offset+pageSize >= total {
			break
		}
	}
	return out
}

// Update 原地更新指定 UUID 的文本记忆正文，并重建其嵌入向量。
//
// 保持条目 ID、角色、来源与入库时间不变；找不到条目返回 ErrNotFound。
func (d *Store) Update(ctx context.Context, id, content string) error {
	id = strings.TrimSpace(id)
	content = strings.TrimSpace(content)
	if id == "" {
		return fmt.Errorf("记忆 ID 不能为空")
	}
	if content == "" {
		return fmt.Errorf("记忆正文不能为空")
	}
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		found := false
		for i := range c.Documents {
			if c.Documents[i].ID != id {
				continue
			}
			if documentType(c.Documents[i]) != TypeText {
				c.mu.RUnlock()
				return fmt.Errorf("只能更新文本记忆: %s", id)
			}
			found = true
			break
		}
		c.mu.RUnlock()
		if !found {
			continue
		}

		vec, err := d.embedText(ctx, currentEmbeddingModel(), content)
		if err != nil {
			return fmt.Errorf("重建嵌入向量失败: %w", err)
		}
		var updated Document
		c.mu.Lock()
		for i := range c.Documents {
			if c.Documents[i].ID == id {
				c.Documents[i].Content = content
				c.Documents[i].Embedding = vec
				updated = c.Documents[i]
				break
			}
		}
		c.mu.Unlock()
		if err := c.saveDocumentsToFile(); err != nil {
			return fmt.Errorf("保存更新失败: %w", err)
		}
		d.keywordIndexAdd(c, updated)
		c.updateLastModTime()
		// 维度校验：Update 改变向量后，确保与索引维度一致
		d.clusterMu.RLock()
		if d.clusterIndex != nil && d.clusterIndex.Dimension > 0 && len(vec) != d.clusterIndex.Dimension {
			logger.Warn("FileManager", "聚类索引：条目 %s 更新后向量维度 %d 与索引维度 %d 不一致",
				id, len(vec), d.clusterIndex.Dimension)
		}
		d.clusterMu.RUnlock()
		return nil
	}
	return ErrNotFound
}

// Delete 删除指定 UUID 的文档（跨全部集合查找）
func (d *Store) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("记忆 ID 不能为空")
	}
	for _, c := range d.snapshotCollections() {
		var deleted Document
		c.mu.Lock()
		found := false
		for i, doc := range c.Documents {
			if doc.ID == id {
				deleted = doc
				c.Documents = append(c.Documents[:i], c.Documents[i+1:]...)
				c.rebuildPositionsLocked()
				found = true
				break
			}
		}
		c.mu.Unlock()
		if !found {
			continue
		}
		if err := c.saveDocumentsToFile(); err != nil {
			return fmt.Errorf("保存文档失败: %w", err)
		}
		d.keywordIndexRemove(id)
		// 删除条目时顺带清理其专属媒体实体（无其它条目引用时）
		d.removeMemoryMediaForDoc(deleted)
		d.detachCluster(id)
		return nil
	}
	return ErrNotFound
}
