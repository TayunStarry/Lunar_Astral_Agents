package module

import (
	"LunarSubsystem/LoggerGeneral"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// UUID v4 生成 — 对应前端 TypeScript UUID() 实现
// 遵循 RFC 4122 版本 4（随机 UUID），格式 xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
// =============================================================================

// generateUUID 生成一个符合 RFC 4122 v4 标准的 UUID 字符串
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		LoggerGeneral.Warn("FileManager", "crypto/rand 失败, 退化为时间戳 UUID: %v", err)
		t := time.Now().UnixNano()
		for i := 0; i < 16; i++ {
			b[i] = byte(t >> (i * 4))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // 版本 4
	b[8] = (b[8] & 0x3f) | 0x80 // 变体 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// =============================================================================
// 初始化 — 记忆库实例与集合管理
// =============================================================================

// initMemoryDBInstance 创建记忆库数据库实例（内部使用）
func initMemoryDBInstance(baseDir string) *MemoryDB {
	return &MemoryDB{
		baseDir:     baseDir,
		collections: make(map[string]*Collection),
	}
}

// MemoryInitInstance 初始化记忆库实例的嵌入服务连接
// 模型配置从 config 模块（lunar_config.json）读取，不再通过参数传入
// 嵌入服务 URL 和 API Key 首次设置后不可变更
func (d *MemoryDB) MemoryInitInstance() error {
	if d.baseDir == "" {
		return fmt.Errorf("记忆库 baseDir 为空，请先调用 InitMemoryDB")
	}

	wasInitialized := d.memoryInitialized

	if !wasInitialized {
		d.httpClient = &http.Client{Timeout: 120 * time.Second}
		d.memoryInitialized = true
	}

	if !wasInitialized {
		d.loadAllCollections()
	}
	return nil
}

// validateCollectionName 验证集合名格式
func (d *MemoryDB) validateCollectionName(name string) error {
	if name == "" {
		return fmt.Errorf("集合名不能为空")
	}
	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return fmt.Errorf("集合名包含非法字符: %s", name)
	}
	return nil
}

// isCollectionValid 检查集合是否合法且已初始化
func (d *MemoryDB) isCollectionValid(name string) error {
	if err := d.validateCollectionName(name); err != nil {
		return err
	}
	if !d.memoryInitialized {
		return fmt.Errorf("记忆库实例未初始化，请先调用 MemoryInitInstance")
	}
	return nil
}

// CollectionInit 初始化或打开一个集合。
// v5: 文本与图片记忆混合存储在同一个集合，不再区分集合类型。
func (d *MemoryDB) CollectionInit(ctx context.Context, name, modelName string) error {
	if err := d.isCollectionValid(name); err != nil {
		return err
	}

	d.collectionsMu.Lock()
	if _, exists := d.collections[name]; exists {
		d.collectionsMu.Unlock()
		// 集合已在缓存，但磁盘目录可能被外部清理；补齐目录，避免后续持久化失败
		if err := os.MkdirAll(filepath.Join(d.baseDir, name), 0755); err != nil {
			return fmt.Errorf("确保集合目录存在失败: %w", err)
		}
		return nil
	}
	d.collectionsMu.Unlock()

	collDir := filepath.Join(d.baseDir, name)
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return fmt.Errorf("创建集合目录失败: %w", err)
	}

	metaPath := filepath.Join(collDir, "metadata.json")

	// 已存在元数据 → 校验版本与嵌入模型；不满足新格式则清空重建（不做旧版迁移）
	recreate := false
	if _, err := os.Stat(metaPath); err == nil {
		var meta collectionMeta
		if err := readJSONFile(metaPath, &meta); err != nil {
			return fmt.Errorf("读取元数据失败: %w", err)
		}
		if meta.Version < CurrentVersion || meta.EmbeddingModel != modelName || meta.EmbeddingDimension == 0 {
			LoggerGeneral.Warn("FileManager", "集合 %s 元数据不满足新格式 (version=%d, model=%s)，清空重建", name, meta.Version, modelName)
			clearCollectionDir(collDir)
			recreate = true
		}
	}

	if !recreate {
		if _, err := os.Stat(metaPath); err == nil {
			// 加载已有集合
			var meta collectionMeta
			if err := readJSONFile(metaPath, &meta); err != nil {
				return fmt.Errorf("读取元数据失败: %w", err)
			}
			c := &Collection{
				Name:      name,
				Model:     modelName,
				Dimension: meta.EmbeddingDimension,
				collDir:   collDir,
				metaPath:  metaPath,
			}
			if err := c.loadDocumentsFromFile(); err != nil {
				return fmt.Errorf("加载文档失败: %w", err)
			}
			c.updateLastModTime()

			d.collectionsMu.Lock()
			d.collections[name] = c
			d.collectionsMu.Unlock()
			return nil
		}
	}

	// 全新创建
	probeDim, err := d.collectionInitProbe(ctx, modelName)
	if err != nil {
		return fmt.Errorf("探针嵌入失败: %w", err)
	}

	c := &Collection{
		Name:      name,
		Model:     modelName,
		Dimension: probeDim,
		Documents: make([]Document, 0),
		collDir:   collDir,
		metaPath:  metaPath,
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

// collectionInitProbe 探针嵌入获取向量维度
func (d *MemoryDB) collectionInitProbe(ctx context.Context, modelName string) (int, error) {
	vec, err := d.embedText(ctx, modelName, "探针文本")
	if err != nil {
		return 0, err
	}
	return len(vec), nil
}

// getCollection 获取集合实例（线程安全）
func (d *MemoryDB) getCollection(name string) (*Collection, error) {
	d.collectionsMu.RLock()
	c, ok := d.collections[name]
	d.collectionsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("集合 '%s' 不存在，请先初始化", name)
	}
	return c, nil
}

// loadAllCollections 从磁盘加载所有集合
func (d *MemoryDB) loadAllCollections() {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		LoggerGeneral.Error("FileManager", "读取记忆库目录失败: %v", err)
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
			LoggerGeneral.Warn("FileManager", "集合 %s 版本过低 (%d < %d)，跳过加载", name, meta.Version, CurrentVersion)
			continue
		}
		if meta.EmbeddingModel == "" || meta.EmbeddingDimension == 0 {
			LoggerGeneral.Warn("FileManager", "集合 %s 元数据不完整，跳过加载", name)
			continue
		}

		c := &Collection{
			Name:      name,
			Model:     meta.EmbeddingModel,
			Dimension: meta.EmbeddingDimension,
			collDir:   filepath.Join(d.baseDir, name),
			metaPath:  metaPath,
		}

		if err := c.loadDocumentsFromFile(); err != nil {
			LoggerGeneral.Error("FileManager", "加载集合 %s 文档失败: %v", name, err)
			continue
		}
		c.updateLastModTime()

		d.collectionsMu.Lock()
		d.collections[name] = c
		d.collectionsMu.Unlock()

		LoggerGeneral.Info("FileManager", "已加载集合 %s (%d 文档)", name, len(c.Documents))
	}
}

// IsMemoryInitialized 检查记忆库是否已初始化
func (d *MemoryDB) IsMemoryInitialized() bool {
	return d.memoryInitialized
}

// =============================================================================
// 文档操作 — 添加、查询、删除
// =============================================================================

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

// findDuplicateByEmbedding 基于内容嵌入向量判定是否与既有记忆重复
// 余弦相似度 > MemoryDedupThreshold 视为重复，返回已存在文档 ID；否则返回空字符串
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
		if cosineSimilarity(vec, doc.Embedding) >= MemoryDedupThreshold {
			return doc.ID
		}
	}
	return ""
}

// MemoryAddMessage 添加文本消息到记忆库。
// 无预设标签时，直接以文本全文计算内容嵌入向量；判定重复仅使用该内容嵌入向量，
// 记忆库中不存储任何标签向量。文本内容存入 content 字段。
func (d *MemoryDB) MemoryAddMessage(ctx context.Context, collectionName, role, content string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	// 1. 嵌入正文全文（文档语义：裸文本，不加前缀）
	contentVec, err := d.embedText(ctx, c.Model, content)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}

	// 2. 仅使用内容嵌入向量判定是否重复
	if existingID := c.findDuplicateByEmbedding(contentVec); existingID != "" {
		LoggerGeneral.Info("FileManager", "集合 %s 已存在内容相近的文本记忆 (ID=%s)，跳过重复写入", collectionName, existingID)
		return existingID, nil
	}

	// 3. 生成 UUID 并写入
	id := generateUUID()
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Role: role, Content: content, Embedding: contentVec, Timestamp: time.Now().Unix()})
	c.mu.Unlock()

	// 4. 持久化
	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}

	return id, nil
}

// MemoryAddMessageWithTags 添加文本消息并携带预设标签。
// 若存在预设标签，将标签按"[标签1],[标签2]"格式拼接后以该文本计算内容嵌入向量；
// 若标签为空，回退为以正文全文计算嵌入向量。文本内容仍存入 content 字段。
// 判定重复仅使用内容嵌入向量。
func (d *MemoryDB) MemoryAddMessageWithTags(ctx context.Context, collectionName, role, content string, tags []string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	// 1. 确定用于嵌入的文本：有预设标签用标签拼接文本，否则用正文全文
	embedTextContent := content
	if len(tags) > 0 {
		embedTextContent = formatTagsText(tags)
	}

	contentVec, err := d.embedText(ctx, c.Model, embedTextContent)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}

	// 2. 仅使用内容嵌入向量判定是否重复
	if existingID := c.findDuplicateByEmbedding(contentVec); existingID != "" {
		LoggerGeneral.Info("FileManager", "集合 %s 已存在内容相近的文本记忆 (ID=%s)，跳过重复写入", collectionName, existingID)
		return existingID, nil
	}

	// 3. 生成 UUID 并写入（content 保存实际文本内容，标签文本仅用于嵌入）
	id := generateUUID()
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Role: role, Content: content, Embedding: contentVec, Timestamp: time.Now().Unix()})
	c.mu.Unlock()

	// 4. 持久化
	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}

	return id, nil
}

// MemoryAddMessageSilent 添加消息但不做重复判定（用于内部导入，纯嵌入入库）
func (d *MemoryDB) MemoryAddMessageSilent(ctx context.Context, collectionName, role, content string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	contentVec, err := d.embedText(ctx, c.Model, content)
	if err != nil {
		return "", fmt.Errorf("内容嵌入失败: %w", err)
	}

	id := generateUUID()
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Role: role, Content: content, Embedding: contentVec, Timestamp: time.Now().Unix()})
	c.mu.Unlock()

	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}
	return id, nil
}

// MemoryAddImage 添加图片到记忆库。
// 调用多模态模型对图片进行标签化理解（仅图片入库时允许生成标签），在单次调用中以一组
// 简短标签的文本形式描述画面内容，将生成的标签按"[标签1],[标签2]"格式拼接后以该文本
// 计算内容嵌入向量（仅存储内容嵌入向量，不存储标签向量）。图片数据存入 base64 字段。
// orientation/custom 指定图片识别取向，为空时默认自动处理。
func (d *MemoryDB) MemoryAddImage(ctx context.Context, collectionName, base64Image string, orientation string, custom string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	// 1. 感知者式标准化：得到入库 URI（静态图缩放/转码；动态图保留动画）与视频理解引用
	storeURI, mediaRefs, animated := standardizeMemoryImage(base64Image)

	// 2. 多模态标签化理解（单线程 + 同步阻塞）：动态图走 file:// 视频链路，静态图走单图
	d.llmMu.Lock()
	var tags []string
	if animated && len(mediaRefs) > 0 {
		tags, err = d.generateTagsFromMedia(ctx, mediaRefs, orientation, custom)
		if err != nil {
			// 视频链路失败（如非 llama-server 后端）：回退单图理解再试一轮
			LoggerGeneral.Warn("FileManager", "动态图视频理解失败（%v），回退单图理解", err)
			tags, err = d.generateTags(ctx, storeURI, true, orientation, custom)
		}
	} else {
		tags, err = d.generateTags(ctx, storeURI, true, orientation, custom)
	}
	d.llmMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("标签生成失败: %w", err)
	}

	// 3. 将标签按[标签1],[标签2]格式拼接，以该文本计算内容嵌入向量
	contentVec, err := d.embedText(ctx, c.Model, formatTagsText(tags))
	if err != nil {
		return "", fmt.Errorf("图片内容嵌入失败: %w", err)
	}

	// 4. 仅使用内容嵌入向量判定是否重复
	if existingID := c.findDuplicateByEmbedding(contentVec); existingID != "" {
		LoggerGeneral.Info("FileManager", "集合 %s 已存在内容相近的图片记忆 (ID=%s)，跳过重复写入", collectionName, existingID)
		return existingID, nil
	}

	// 5. 添加文档（base64 存储图片，内容字段为空，仅存储内容嵌入向量）
	id := generateUUID()
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Base64: storeURI, Embedding: contentVec, Timestamp: time.Now().Unix()})
	c.mu.Unlock()

	// 6. 持久化
	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}

	return id, nil
}

// MemoryQueryMessages 查询记忆库，返回 topK 条最匹配的 JSON 消息字符串
func (d *MemoryDB) MemoryQueryMessages(ctx context.Context, collectionName, queryText string, topK int) ([]string, error) {
	results, err := d.MemoryQueryMessagesWithContent(ctx, collectionName, queryText, topK)
	if err != nil {
		return nil, err
	}

	jsonMessages := make([]string, 0, len(results))
	for _, r := range results {
		msg := memoryMessage{
			Role:      r.Role,
			Content:   r.Content,
			Base64:    r.Base64,
			Timestamp: r.Timestamp,
		}
		if r.Base64 != "" {
			msg.Role = "image"
		}
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		jsonMessages = append(jsonMessages, string(jsonBytes))
	}
	return jsonMessages, nil
}

// MemoryQueryMessagesWithContent 查询记忆库，返回 topK 条带内容的查询结果。
// v5: 内容向量检索——嵌入查询文本，与全量文档内容向量算余弦，过滤后降序返回。
// 客户端依据返回结果的 content 与 base64 字段自行判定和选取记忆类型。
func (d *MemoryDB) MemoryQueryMessagesWithContent(ctx context.Context, collectionName, queryText string, topK int) ([]MemoryQueryResult, error) {
	if topK <= 0 {
		return nil, nil
	}

	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	c.reloadIfChanged()

	// 1. 嵌入查询文本（query 语义：Qwen3 指令感知前缀）
	queryVec, err := d.embedQuery(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("查询嵌入失败: %w", err)
	}

	// 2. 内容向量检索
	return c.queryTopK(queryVec, topK), nil
}

// queryTopK 内容向量检索核心算法 (v5)
// 嵌入查询 → 与全量文档内容向量算余弦相似度 → 过滤低于 DocRankThreshold 的 →
// 按相似度降序（同分按插入顺序）返回 topK 条
func (c *Collection) queryTopK(queryVec []float32, topK int) []MemoryQueryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Documents) == 0 || topK <= 0 {
		return nil
	}

	type ranked struct {
		idx   int
		sim   float32
		order int
	}
	rankedList := make([]ranked, 0, len(c.Documents))
	for i, doc := range c.Documents {
		if len(doc.Embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, doc.Embedding)
		if sim < DocRankThreshold {
			continue
		}
		rankedList = append(rankedList, ranked{idx: i, sim: sim, order: i})
	}

	if len(rankedList) == 0 {
		return nil
	}

	sort.SliceStable(rankedList, func(i, j int) bool {
		if rankedList[i].sim != rankedList[j].sim {
			return rankedList[i].sim > rankedList[j].sim
		}
		return rankedList[i].order < rankedList[j].order
	})

	if topK > len(rankedList) {
		topK = len(rankedList)
	}

	results := make([]MemoryQueryResult, topK)
	for i := 0; i < topK; i++ {
		doc := &c.Documents[rankedList[i].idx]
		role := doc.Role
		if doc.Base64 != "" {
			role = "image"
		}
		results[i] = MemoryQueryResult{
			ID:         doc.ID,
			Role:       role,
			Content:    doc.Content,
			Base64:     doc.Base64,
			Similarity: rankedList[i].sim,
			Timestamp:  doc.Timestamp,
		}
	}
	return results
}

// MemoryDeleteMessage 删除指定 UUID 的文档
func (d *MemoryDB) MemoryDeleteMessage(ctx context.Context, collectionName, id string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	// 1. 从文档列表中删除
	c.mu.Lock()
	found := false
	for i, doc := range c.Documents {
		if doc.ID == id {
			c.Documents = append(c.Documents[:i], c.Documents[i+1:]...)
			found = true
			break
		}
	}
	c.mu.Unlock()

	if !found {
		return nil
	}

	// 2. 持久化
	if err := c.saveDocumentsToFile(); err != nil {
		return fmt.Errorf("保存文档失败: %w", err)
	}

	return nil
}

// =============================================================================
// 集合信息查询 — 计数、列表、元数据
// =============================================================================

// MemoryGetCollectionCount 返回所有集合的数量
func (d *MemoryDB) MemoryGetCollectionCount() int {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	return len(d.collections)
}

// MemoryGetDocuments 获取集合中文档的分页列表
func (d *MemoryDB) MemoryGetDocuments(collectionName string, offset, limit int) ([]DocumentEntry, int) {
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
		role := doc.Role
		if doc.Base64 != "" {
			role = "image"
		}
		entries[i-offset] = DocumentEntry{
			ID:        doc.ID,
			Role:      role,
			Content:   doc.Content,
			Base64:    doc.Base64,
			Timestamp: doc.Timestamp,
		}
	}
	return entries, total
}

// MemoryGetEntryCount 返回指定集合的文档总数
func (d *MemoryDB) MemoryGetEntryCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	return c.docCount()
}

// MemoryHasSyncMismatch 检查集合中是否存在缺失内容向量的文档
func (d *MemoryDB) MemoryHasSyncMismatch(collectionName string) bool {
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

// MemoryDeleteCollection 删除整个集合及其所有数据
func (d *MemoryDB) MemoryDeleteCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	d.collectionsMu.Lock()
	delete(d.collections, collectionName)
	d.collectionsMu.Unlock()

	if err := os.RemoveAll(c.collDir); err != nil {
		return fmt.Errorf("删除集合目录失败: %w", err)
	}
	return nil
}

// MemoryClearCollection 清空集合所有数据，保留 metadata.json
func (d *MemoryDB) MemoryClearCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

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

	c.documentsChunkCount = 0
	c.embeddingsChunkCount = 0

	return c.saveCollectionMeta()
}

// MemoryRebuildEntries 重建集合格文档的内容嵌入向量：
// 文本记忆以正文重新嵌入；图片记忆重新调用多模态生成标签并以标签文本重新嵌入。
// 当嵌入模型变更时调用。
func (d *MemoryDB) MemoryRebuildEntries(ctx context.Context, collectionName, modelName string, progress func(int, int)) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	c.mu.RLock()
	docs := make([]Document, len(c.Documents))
	copy(docs, c.Documents)
	c.mu.RUnlock()

	// 更新模型（modelName 为空时沿用当前模型）
	effectiveModel := c.Model
	if modelName != "" {
		effectiveModel = modelName
		c.Model = modelName
	}

	for i, doc := range docs {
		if progress != nil {
			progress(i+1, len(docs))
		}

		var contentVec []float32
		if doc.Base64 != "" {
			// 图片记忆：重新多模态生成标签并以标签文本嵌入
			d.llmMu.Lock()
			tags, terr := d.generateTags(ctx, doc.Base64, true, RecognitionAuto, "")
			d.llmMu.Unlock()
			if terr != nil {
				LoggerGeneral.Warn("FileManager", "重建图片标签失败 [%s]: %v", doc.ID, terr)
				continue
			}
			contentVec, err = d.embedText(ctx, effectiveModel, formatTagsText(tags))
		} else {
			// 文本记忆：以正文全文重新嵌入
			contentVec, err = d.embedText(ctx, effectiveModel, doc.Content)
		}
		if err != nil {
			LoggerGeneral.Warn("FileManager", "嵌入内容失败 [%s]: %v", doc.ID, err)
			continue
		}

		c.mu.Lock()
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
	if err := c.saveCollectionMeta(); err != nil {
		return fmt.Errorf("保存元数据失败: %w", err)
	}

	return nil
}

// MemoryListCollections 返回所有集合名称
func (d *MemoryDB) MemoryListCollections() []string {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()

	names := make([]string, 0, len(d.collections))
	for name := range d.collections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MemoryGetCollectionInfo 返回指定集合的详细元数据信息
func (d *MemoryDB) MemoryGetCollectionInfo(collectionName string) map[string]interface{} {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"name":                  c.Name,
		"embedding_model":       c.Model,
		"embedding_dimension":   c.Dimension,
		"version":               CurrentVersion,
		"document_count":        len(c.Documents),
		"documents_chunk_count": c.documentsChunkCount,
	}
}

// MemoryGetCollectionInfoWithType 返回指定集合的详细元数据信息（含类型，兼容历史调用方）
func (d *MemoryDB) MemoryGetCollectionInfoWithType(collectionName string) map[string]interface{} {
	return d.MemoryGetCollectionInfo(collectionName)
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
// JSON 文件 I/O 工具
// =============================================================================

// readJSONFile 读取 JSON 文件并反序列化到目标结构体
func readJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// atomicWriteJSON 原子写入 JSON 文件（先写临时文件，再重命名；单行紧凑存储，不格式化）
func atomicWriteJSON(path string, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	// 确保目标目录存在：集合目录可能被外部清理或重建遗漏，缺失时自动创建，避免写临时文件失败
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("确保目录存在失败: %w", err)
		}
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// Windows 上目标文件（尤其高频写的 metadata.json）可能被瞬时占用（并发读/AV 扫描/连续 rename），
	// 直接 rename 会偶发 "Access is denied"。先短暂重试，再尝试移除目标后重命名。
	if err := substituteRename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// substituteRename 替换式重命名：短暂重试 + 移除目标兜底，规避 Windows 目标文件占用导致 Access denied
func substituteRename(tmpPath, path string) error {
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.Rename(tmpPath, path); err == nil {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	// 兜底：先移除旧目标再重命名
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err == nil {
		return nil
	}
	return fmt.Errorf("rename %s %s", tmpPath, path)
}

// =============================================================================
// 持久化 — 元数据、文档
// =============================================================================

// saveCollectionMeta 保存集合元数据到 metadata.json
func (c *Collection) saveCollectionMeta() error {
	meta := collectionMeta{
		EmbeddingModel:       c.Model,
		EmbeddingDimension:   c.Dimension,
		Version:              CurrentVersion,
		DocumentsChunkCount:  c.documentsChunkCount,
		EmbeddingsChunkCount: c.embeddingsChunkCount,
	}
	return atomicWriteJSON(c.metaPath, meta)
}

// updateLastModTime 更新最后修改时间戳
func (c *Collection) updateLastModTime() {
	if info, err := os.Stat(c.metaPath); err == nil {
		c.lastFileModTime = info.ModTime()
	}
}

// loadDocumentsFromFile 从分块文件加载文档与嵌入向量到内存
// documents_NNNN.json 存文档本体（不含向量），embeddings_NNNN.json 存向量并按 ID 关联
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
	}

	c.mu.Lock()
	c.Documents = allDocs
	c.mu.Unlock()

	return nil
}

// saveDocumentsToFile 将文档与嵌入向量保存到分块文件（100 条/块）
// 文档写入 documents_NNNN.json（Embedding 为 json:"-" 不落盘），
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

	c.documentsChunkCount = newChunkCount
	c.embeddingsChunkCount = newChunkCount

	return c.saveCollectionMeta()
}

// reloadIfChanged 检测 metadata.json 是否被外部进程修改，若是则重新加载
func (c *Collection) reloadIfChanged() {
	if info, err := os.Stat(c.metaPath); err != nil {
		return
	} else if !info.ModTime().Equal(c.lastFileModTime) {
		c.loadDocumentsFromFile()
		c.updateLastModTime()
	}
}

// =============================================================================
// 余弦相似度
// =============================================================================

// cosineSimilarity 计算两个向量的余弦相似度
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return float32(dotProduct / denom)
}

// =============================================================================
// 全局包装函数 — 供外部模块调用
// =============================================================================

var globalMemoryDB *MemoryDB

// InitMemoryDB 全局初始化（幂等：若已初始化则直接返回现有实例）
func InitMemoryDB(baseDir string) *MemoryDB {
	if globalMemoryDB != nil {
		return globalMemoryDB
	}
	globalMemoryDB = &MemoryDB{
		baseDir:     baseDir,
		collections: make(map[string]*Collection),
	}
	return globalMemoryDB
}

// GetMemoryDB 获取全局 MemoryDB 实例
func GetMemoryDB() *MemoryDB {
	return globalMemoryDB
}

// MemoryInitInstance 全局初始化记忆库实例（模型配置从 config 模块读取）
func MemoryInitInstance() error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryInitInstance()
}

// CollectionInit 全局初始化集合
func CollectionInit(ctx context.Context, name, modelName string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.CollectionInit(ctx, name, modelName)
}

// MemoryAddMessage 全局添加消息
func MemoryAddMessage(ctx context.Context, collectionName, role, content string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessage(ctx, collectionName, role, content)
}

// MemoryAddMessageSilent 全局添加消息（无重复判定）
func MemoryAddMessageSilent(ctx context.Context, collectionName, role, content string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessageSilent(ctx, collectionName, role, content)
}

// MemoryAddMessageWithTags 全局添加消息（携带预设标签，以标签文本嵌入）
func MemoryAddMessageWithTags(ctx context.Context, collectionName, role, content string, tags []string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessageWithTags(ctx, collectionName, role, content, tags)
}

// MemoryAddImage 全局添加图片
func MemoryAddImage(ctx context.Context, collectionName, base64Image string, orientation string, custom string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddImage(ctx, collectionName, base64Image, orientation, custom)
}

// MemoryQueryMessages 全局查询消息
func MemoryQueryMessages(ctx context.Context, collectionName, queryText string, topK int) ([]string, error) {
	if globalMemoryDB == nil {
		return nil, fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryQueryMessages(ctx, collectionName, queryText, topK)
}

// MemoryQueryMessagesWithContent 全局查询消息（带内容）
func MemoryQueryMessagesWithContent(ctx context.Context, collectionName, queryText string, topK int) ([]MemoryQueryResult, error) {
	if globalMemoryDB == nil {
		return nil, fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryQueryMessagesWithContent(ctx, collectionName, queryText, topK)
}

// MemoryDeleteMessage 全局删除消息
func MemoryDeleteMessage(ctx context.Context, collectionName, id string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryDeleteMessage(ctx, collectionName, id)
}

// MemoryGetCollectionCount 全局获取集合数
func MemoryGetCollectionCount() int {
	if globalMemoryDB == nil {
		return 0
	}
	return globalMemoryDB.MemoryGetCollectionCount()
}

// MemoryGetDocuments 全局获取文档分页
func MemoryGetDocuments(collectionName string, offset, limit int) ([]DocumentEntry, int) {
	if globalMemoryDB == nil {
		return nil, 0
	}
	return globalMemoryDB.MemoryGetDocuments(collectionName, offset, limit)
}

// MemoryGetEntryCount 全局获取文档数
func MemoryGetEntryCount(collectionName string) int {
	if globalMemoryDB == nil {
		return 0
	}
	return globalMemoryDB.MemoryGetEntryCount(collectionName)
}

// MemoryHasSyncMismatch 全局检查同步不一致
func MemoryHasSyncMismatch(collectionName string) bool {
	if globalMemoryDB == nil {
		return false
	}
	return globalMemoryDB.MemoryHasSyncMismatch(collectionName)
}

// MemoryDeleteCollection 全局删除集合
func MemoryDeleteCollection(collectionName string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryDeleteCollection(collectionName)
}

// MemoryClearCollection 全局清空集合
func MemoryClearCollection(collectionName string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryClearCollection(collectionName)
}

// MemoryRebuildEntries 全局重建集合
func MemoryRebuildEntries(ctx context.Context, collectionName, modelName string, progress func(int, int)) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryRebuildEntries(ctx, collectionName, modelName, progress)
}

// MemoryListCollections 全局列出集合
func MemoryListCollections() []string {
	if globalMemoryDB == nil {
		return nil
	}
	return globalMemoryDB.MemoryListCollections()
}

// MemoryGetCollectionInfo 全局获取集合信息
func MemoryGetCollectionInfo(collectionName string) map[string]interface{} {
	if globalMemoryDB == nil {
		return nil
	}
	return globalMemoryDB.MemoryGetCollectionInfo(collectionName)
}

// MemoryGetCollectionInfoWithType 全局获取集合信息（含类型）
func MemoryGetCollectionInfoWithType(collectionName string) map[string]interface{} {
	if globalMemoryDB == nil {
		return nil
	}
	return globalMemoryDB.MemoryGetCollectionInfoWithType(collectionName)
}

// IsMemoryInitialized 全局检查记忆库初始化状态
func IsMemoryInitialized() bool {
	if globalMemoryDB == nil {
		return false
	}
	return globalMemoryDB.IsMemoryInitialized()
}
