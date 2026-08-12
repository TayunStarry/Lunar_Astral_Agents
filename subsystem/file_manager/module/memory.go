package module

import (
	"LunarSubsystem/GeneralConfig"
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

// MemoryInitInstance 初始化记忆库实例的嵌入服务与 LLM 标签生成服务连接
// 模型配置从 config 模块（lunar_config.json）读取，不再通过参数传入
// 嵌入服务 URL 和 API Key 首次设置后不可变更；LLM 配置可后续通过 config 更新
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

// CollectionInit 初始化或打开一个集合，验证版本与配置一致性
// collectionType: "text" 或 "image"
func (d *MemoryDB) CollectionInit(ctx context.Context, name, modelName, collectionType string) error {
	if err := d.isCollectionValid(name); err != nil {
		return err
	}

	d.collectionsMu.Lock()
	if _, exists := d.collections[name]; exists {
		d.collectionsMu.Unlock()
		return nil
	}
	d.collectionsMu.Unlock()

	collDir := filepath.Join(d.baseDir, name)
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return fmt.Errorf("创建集合目录失败: %w", err)
	}

	metaPath := filepath.Join(collDir, "metadata.json")

	if _, err := os.Stat(metaPath); err == nil {
		// 元数据存在 → 验证
		var meta collectionMeta
		if err := readJSONFile(metaPath, &meta); err != nil {
			return fmt.Errorf("读取元数据失败: %w", err)
		}
		return d.collectionInitFromMeta(ctx, name, modelName, collectionType, &meta, collDir, metaPath)
	}

	// 全新创建
	return d.collectionInitNew(ctx, name, modelName, collectionType, collDir, metaPath)
}

// collectionInitFromMeta 从已有 metadata.json 验证并初始化集合
func (d *MemoryDB) collectionInitFromMeta(ctx context.Context, name, modelName, collectionType string, meta *collectionMeta, collDir, metaPath string) error {
	needRebuild := false

	// 版本缺失或过低
	if meta.Version < CurrentVersion {
		LoggerGeneral.Warn("FileManager", "集合 %s 版本 %d < %d，触发重建", name, meta.Version, CurrentVersion)
		needRebuild = true
	}

	// 嵌入模型缺失或不匹配
	if meta.EmbeddingModel == "" || meta.EmbeddingModel != modelName {
		LoggerGeneral.Warn("FileManager", "集合 %s 嵌入模型不匹配 [%s] vs [%s]，触发重建", name, meta.EmbeddingModel, modelName)
		needRebuild = true
	}

	// 嵌入维度缺失
	if meta.EmbeddingDimension == 0 {
		probeDim, err := d.collectionInitProbe(ctx, modelName)
		if err != nil {
			return fmt.Errorf("探针嵌入失败: %w", err)
		}
		if meta.EmbeddingDimension != 0 && meta.EmbeddingDimension != probeDim {
			LoggerGeneral.Warn("FileManager", "集合 %s 嵌入维度不匹配 [%d] vs [%d]，触发重建", name, meta.EmbeddingDimension, probeDim)
			needRebuild = true
		}
		meta.EmbeddingDimension = probeDim
	}

	// 类型不匹配
	if meta.Type != "" && meta.Type != collectionType {
		return fmt.Errorf("集合类型不匹配: 请求 %s, 已有 %s", collectionType, meta.Type)
	}

	if needRebuild {
		return d.collectionInitRebuild(ctx, name, modelName, collectionType, collDir, metaPath)
	}

	// 全部通过 → 加载数据
	c := &Collection{
		Name:            name,
		Model:           modelName,
		Dimension:       meta.EmbeddingDimension,
		CollectionType:  collectionType,
		MultimodalModel: *GeneralConfig.MemoryMultimodalModel,
		collDir:         collDir,
		metaPath:        metaPath,
	}

	if err := c.loadDocumentsFromFile(); err != nil {
		return fmt.Errorf("加载文档失败: %w", err)
	}
	if err := c.loadTagsFromFile(); err != nil {
		return fmt.Errorf("加载标签向量失败: %w", err)
	}
	c.updateLastModTime()

	d.collectionsMu.Lock()
	d.collections[name] = c
	d.collectionsMu.Unlock()

	return nil
}

// collectionInitNew 全新创建集合
func (d *MemoryDB) collectionInitNew(ctx context.Context, name, modelName, collectionType, collDir, metaPath string) error {
	probeDim, err := d.collectionInitProbe(ctx, modelName)
	if err != nil {
		return fmt.Errorf("探针嵌入失败: %w", err)
	}

	c := &Collection{
		Name:            name,
		Model:           modelName,
		Dimension:       probeDim,
		CollectionType:  collectionType,
		MultimodalModel: *GeneralConfig.MemoryMultimodalModel,
		Documents:       make([]Document, 0),
		TagVectors:      make([]TagVector, 0),
		collDir:         collDir,
		metaPath:        metaPath,
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

// collectionInitRebuild 清空旧数据并重建集合
func (d *MemoryDB) collectionInitRebuild(ctx context.Context, name, modelName, collectionType, collDir, metaPath string) error {
	LoggerGeneral.Warn("FileManager", "集合 %s 触发重建，清空所有旧数据", name)

	// 删除所有旧数据文件
	patterns := []string{"documents_*.json", "images_*.json", "tags_*.json", "contents_*.json", "embeddings_*.json", "base64_*.json", "documents.json"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(collDir, pattern))
		for _, f := range files {
			os.Remove(f)
		}
	}

	// 重新创建
	return d.collectionInitNew(ctx, name, modelName, collectionType, collDir, metaPath)
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

		// v2 版本检查
		if meta.Version < CurrentVersion {
			LoggerGeneral.Warn("FileManager", "集合 %s 版本过低 (%d < %d)，跳过加载", name, meta.Version, CurrentVersion)
			continue
		}
		if meta.EmbeddingModel == "" || meta.EmbeddingDimension == 0 {
			LoggerGeneral.Warn("FileManager", "集合 %s 元数据不完整，跳过加载", name)
			continue
		}

		collType := meta.Type
		if collType == "" {
			collType = CollectionTypeText
		}

		collDir := filepath.Join(d.baseDir, name)
		c := &Collection{
			Name:            name,
			Model:           meta.EmbeddingModel,
			Dimension:       meta.EmbeddingDimension,
			CollectionType:  collType,
			MultimodalModel: meta.MultimodalModel,
			collDir:         collDir,
			metaPath:        metaPath,
		}

		if err := c.loadDocumentsFromFile(); err != nil {
			LoggerGeneral.Error("FileManager", "加载集合 %s 文档失败: %v", name, err)
			continue
		}
		if err := c.loadTagsFromFile(); err != nil {
			LoggerGeneral.Error("FileManager", "加载集合 %s 标签向量失败: %v", name, err)
			continue
		}
		c.updateLastModTime()

		d.collectionsMu.Lock()
		d.collections[name] = c
		d.collectionsMu.Unlock()

		LoggerGeneral.Info("FileManager", "已加载集合 %s (%s, %d 文档, %d 标签)", name, collType, len(c.Documents), len(c.TagVectors))
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

// MemoryAddMessage 添加文本消息到记忆库，同步阻塞等待 LLM 标签生成完成
// 返回生成的文档 UUID，LLM 标签生成失败则不存储文档
// v3: 文档存储 TagUUIDs，标签向量不再存储文档引用
func (d *MemoryDB) MemoryAddMessage(ctx context.Context, collectionName, role, content string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}
	if c.CollectionType != CollectionTypeText {
		return "", fmt.Errorf("集合 %s 类型为 %s，不支持文本消息添加", collectionName, c.CollectionType)
	}

	// 1. 生成 UUID
	id := generateUUID()

	// 2. LLM 生成标签（单线程 + 同步阻塞）
	d.llmMu.Lock()
	tags, err := d.generateTags(ctx, content, false)
	d.llmMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("标签生成失败: %w", err)
	}

	// 3. 嵌入标签
	tagVecs, err := d.embedTexts(ctx, c.Model, tags)
	if err != nil {
		return "", fmt.Errorf("标签嵌入失败: %w", err)
	}

	// 4. v3: 去重匹配，获取标签 UUID
	tagUUIDs := c.processTagVectors(tags, tagVecs)

	// 5. 添加文档（含 TagUUIDs）
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Role: role, Content: content, TagUUIDs: tagUUIDs})
	c.mu.Unlock()

	// 6. 持久化
	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}
	if err := c.saveTagsToFile(); err != nil {
		return "", fmt.Errorf("保存标签向量失败: %w", err)
	}

	return id, nil
}

// MemoryAddMessageSilent 添加消息但不生成标签（用于内部导入，无 LLM 开销）
func (d *MemoryDB) MemoryAddMessageSilent(ctx context.Context, collectionName, role, content string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}
	if c.CollectionType != CollectionTypeText {
		return "", fmt.Errorf("集合 %s 类型为 %s，不支持文本消息添加", collectionName, c.CollectionType)
	}

	id := generateUUID()
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Role: role, Content: content})
	c.mu.Unlock()

	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}
	return id, nil
}

// MemoryAddImage 添加图片到记忆库，同步阻塞等待 LLM 标签生成完成
// base64Image 为完整的 data:image/...;base64,... 格式
// v3: 文档存储 TagUUIDs，标签向量不再存储文档引用
func (d *MemoryDB) MemoryAddImage(ctx context.Context, collectionName, base64Image string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}
	if c.CollectionType != CollectionTypeImage {
		return "", fmt.Errorf("集合 %s 类型为 %s，不支持图片添加", collectionName, c.CollectionType)
	}

	// 1. 生成 UUID
	id := generateUUID()

	// 2. LLM 生成标签（单线程 + 同步阻塞）
	d.llmMu.Lock()
	tags, err := d.generateTags(ctx, base64Image, true)
	d.llmMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("标签生成失败: %w", err)
	}

	// 3. 嵌入标签
	tagVecs, err := d.embedTexts(ctx, c.Model, tags)
	if err != nil {
		return "", fmt.Errorf("标签嵌入失败: %w", err)
	}

	// 4. v3: 去重匹配，获取标签 UUID
	tagUUIDs := c.processTagVectors(tags, tagVecs)

	// 5. 添加文档（含 TagUUIDs）
	c.mu.Lock()
	c.Documents = append(c.Documents, Document{ID: id, Image: base64Image, TagUUIDs: tagUUIDs})
	c.mu.Unlock()

	// 6. 持久化
	if err := c.saveDocumentsToFile(); err != nil {
		return "", fmt.Errorf("保存文档失败: %w", err)
	}
	if err := c.saveTagsToFile(); err != nil {
		return "", fmt.Errorf("保存标签向量失败: %w", err)
	}

	return id, nil
}

// processTagVectors 对标签向量进行去重匹配，返回每个标签对应的 UUID
// v3: 标签向量拥有独立 UUID，不再存储文档引用
// 余弦相似度 > TagDedupThreshold 时复用已有标签向量 UUID
func (c *Collection) processTagVectors(tags []string, newVecs [][]float32) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	tagUUIDs := make([]string, len(newVecs))
	for i, vec := range newVecs {
		bestIdx := -1
		bestSim := float32(-1)

		for j, tv := range c.TagVectors {
			sim := cosineSimilarity(vec, tv.Embedding)
			if sim > bestSim {
				bestSim = sim
				bestIdx = j
			}
		}

		if bestSim >= TagDedupThreshold {
			// 复用已有标签向量
			tagUUIDs[i] = c.TagVectors[bestIdx].UUID
		} else {
			// 新增标签向量
			tagText := ""
			if i < len(tags) {
				tagText = tags[i]
			}
			newUUID := generateUUID()
			c.TagVectors = append(c.TagVectors, TagVector{
				UUID:      newUUID,
				Tag:       tagText,
				Embedding: vec,
			})
			tagUUIDs[i] = newUUID
		}
	}
	return tagUUIDs
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
			Role:    r.Role,
			Content: r.Content,
		}
		if r.Image != "" {
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

// MemoryQueryMessagesWithContent 查询记忆库，返回 topK 条带内容的查询结果
// 使用标签向量中介检索算法：
//  1. 嵌入查询文本
//  2. 与所有标签向量计算余弦相似度
//  3. 取 topK 个最相似标签
//  4. 收集关联 UUID 并统计频次
//  5. 得分 = 频次 / 去重 UUID 总数
//  6. 按得分降序 + 原始插入顺序返回
func (d *MemoryDB) MemoryQueryMessagesWithContent(ctx context.Context, collectionName, queryText string, topK int) ([]MemoryQueryResult, error) {
	if topK <= 0 {
		return nil, nil
	}

	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	c.reloadIfChanged()

	// 1. 嵌入查询文本
	queryVec, err := d.embedText(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("查询嵌入失败: %w", err)
	}

	// 2-6. 标签向量匹配查询
	return c.queryTopK(queryVec, topK), nil
}

// queryTopK 标签向量中介检索核心算法 (v3: 文档引用标签 UUID)
// 流程: 嵌入查询 → 匹配标签 → 取标签 UUID → 筛选文档 → 平均评分排序
// 评分: 所有匹配标签的余弦相似度取平均值（多标签命中取均分）
// 例如: 文档A被标签1(0.5)和标签2(0.8)同时命中 → 得分 (0.5+0.8)/2 = 0.65
func (c *Collection) queryTopK(queryVec []float32, topK int) []MemoryQueryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.TagVectors) == 0 || len(c.Documents) == 0 || topK <= 0 {
		return nil
	}

	// 步骤 1: 计算所有标签向量的相似度并取 topK
	type tagScored struct {
		idx   int
		score float32
	}
	tagScores := make([]tagScored, len(c.TagVectors))
	for i, tv := range c.TagVectors {
		tagScores[i] = tagScored{idx: i, score: cosineSimilarity(queryVec, tv.Embedding)}
	}

	sort.SliceStable(tagScores, func(i, j int) bool {
		return tagScores[i].score > tagScores[j].score
	})

	actualTagK := topK
	if actualTagK > len(tagScores) {
		actualTagK = len(tagScores)
	}

	// 步骤 2: 构建 标签UUID → 相似度 映射（topK 标签）
	tagScoreMap := make(map[string]float32, actualTagK)
	for i := 0; i < actualTagK; i++ {
		uuid := c.TagVectors[tagScores[i].idx].UUID
		// 同一 UUID 可能被多个标签命中（去重后不存在），取最高分
		if existing, ok := tagScoreMap[uuid]; !ok || tagScores[i].score > existing {
			tagScoreMap[uuid] = tagScores[i].score
		}
	}

	// 步骤 3: 筛选包含这些标签的文档，计算匹配标签的余弦相似度平均值
	type docScored struct {
		doc      Document
		avgScore float32
		order    int
	}

	docOrder := make(map[string]int)
	for i, doc := range c.Documents {
		docOrder[doc.ID] = i
	}

	docScores := make([]docScored, 0)
	for _, doc := range c.Documents {
		var sum float32
		var matchCount int
		for _, tagUUID := range doc.TagUUIDs {
			if score, ok := tagScoreMap[tagUUID]; ok {
				sum += score
				matchCount++
			}
		}
		if matchCount > 0 {
			docScores = append(docScores, docScored{
				doc:      doc,
				avgScore: sum / float32(matchCount),
				order:    docOrder[doc.ID],
			})
		}
	}

	if len(docScores) == 0 {
		return nil
	}

	// 步骤 4: 按平均得分降序排列，平局按原始插入顺序
	sort.SliceStable(docScores, func(i, j int) bool {
		if docScores[i].avgScore != docScores[j].avgScore {
			return docScores[i].avgScore > docScores[j].avgScore
		}
		return docScores[i].order < docScores[j].order
	})

	// 步骤 5: 返回前 topK 条
	if topK > len(docScores) {
		topK = len(docScores)
	}

	results := make([]MemoryQueryResult, topK)
	for i := 0; i < topK; i++ {
		doc := &docScores[i].doc
		role := doc.Role
		if doc.Image != "" {
			role = "image"
		}
		results[i] = MemoryQueryResult{
			ID:         doc.ID,
			Role:       role,
			Content:    doc.Content,
			Image:      doc.Image,
			Similarity: docScores[i].avgScore,
		}
	}
	return results
}

// MemoryDeleteMessage 删除指定 UUID 的文档
// v3: 仅删除文档本身 (O(1))，不再遍历标签向量清理引用
// 悬空标签留待 MemoryRebuildEntries 或 MemoryClearCollection 时统一清理
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

	// 2. v3: 仅持久化文档，不清理标签向量（悬空标签留待重建时处理）
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
		if doc.Image != "" {
			role = "image"
		}
		entries[i-offset] = DocumentEntry{
			ID:      doc.ID,
			Role:    role,
			Content: doc.Content,
			Image:   doc.Image,
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

// MemoryHasSyncMismatch 检查集合中文档引用的标签 UUID 是否都存在
// v3: 检查文档的 TagUUIDs 是否都指向存在的标签向量
func (d *MemoryDB) MemoryHasSyncMismatch(collectionName string) bool {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return false
	}

	c.reloadIfChanged()

	c.mu.RLock()
	defer c.mu.RUnlock()

	docCount := len(c.Documents)
	if docCount == 0 {
		return false
	}

	tagCount := len(c.TagVectors)
	if tagCount == 0 && docCount > 0 {
		return true // 有文档但没有标签向量
	}

	// v3: 检查是否有文档引用了不存在的标签 UUID
	tagUUIDSet := make(map[string]struct{}, tagCount)
	for _, tv := range c.TagVectors {
		tagUUIDSet[tv.UUID] = struct{}{}
	}
	for _, doc := range c.Documents {
		for _, tagUUID := range doc.TagUUIDs {
			if _, ok := tagUUIDSet[tagUUID]; !ok {
				return true // 文档引用了不存在的标签（悬空引用）
			}
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
	c.TagVectors = make([]TagVector, 0)
	c.mu.Unlock()

	// 删除所有分块文件
	patterns := []string{"documents_*.json", "images_*.json", "tags_*.json"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(c.collDir, pattern))
		for _, f := range files {
			os.Remove(f)
		}
	}

	c.documentsChunkCount = 0
	c.imagesChunkCount = 0
	c.tagsChunkCount = 0

	return c.saveCollectionMeta()
}

// MemoryRebuildEntries 重建集合的标签向量（重新生成标签 + 嵌入）
// v3: 逐文档重新生成标签，仅保留被文档引用的标签向量（自动丢弃悬空标签）
// 当嵌入模型变更时调用
func (d *MemoryDB) MemoryRebuildEntries(ctx context.Context, collectionName, modelName string, progress func(int, int)) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	c.mu.RLock()
	docs := make([]Document, len(c.Documents))
	copy(docs, c.Documents)
	c.mu.RUnlock()

	// v3: 清空旧标签向量，重建后仅保留被引用的标签
	c.mu.Lock()
	c.TagVectors = make([]TagVector, 0)
	c.mu.Unlock()

	// 更新模型
	c.Model = modelName

	for i, doc := range docs {
		if progress != nil {
			progress(i+1, len(docs))
		}

		var content string
		var isImage bool
		if doc.Image != "" {
			content = doc.Image
			isImage = true
		} else {
			content = doc.Content
		}

		// LLM 生成标签
		d.llmMu.Lock()
		tags, err := d.generateTags(ctx, content, isImage)
		d.llmMu.Unlock()
		if err != nil {
			LoggerGeneral.Warn("FileManager", "重建标签失败 [%s]: %v", doc.ID, err)
			continue
		}

		// 嵌入标签
		tagVecs, err := d.embedTexts(ctx, modelName, tags)
		if err != nil {
			LoggerGeneral.Warn("FileManager", "嵌入标签失败 [%s]: %v", doc.ID, err)
			continue
		}

		// v3: 去重匹配，获取标签 UUID
		tagUUIDs := c.processTagVectors(tags, tagVecs)

		// v3: 更新文档的 TagUUIDs
		c.mu.Lock()
		for j := range c.Documents {
			if c.Documents[j].ID == doc.ID {
				c.Documents[j].TagUUIDs = tagUUIDs
				break
			}
		}
		c.mu.Unlock()
	}

	// v3: 重建完成，TagVectors 仅包含被文档引用的标签（悬空标签已自动丢弃）
	if err := c.saveDocumentsToFile(); err != nil {
		return fmt.Errorf("保存文档失败: %w", err)
	}
	if err := c.saveTagsToFile(); err != nil {
		return fmt.Errorf("保存标签向量失败: %w", err)
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
		"multimodal_model":      c.MultimodalModel,
		"type":                  c.CollectionType,
		"version":               CurrentVersion,
		"document_count":        len(c.Documents),
		"tag_count":             len(c.TagVectors),
		"documents_chunk_count": c.documentsChunkCount,
		"images_chunk_count":    c.imagesChunkCount,
		"tags_chunk_count":      c.tagsChunkCount,
	}
}

// MemoryGetCollectionInfoWithType 返回指定集合的详细元数据信息（含类型）
func (d *MemoryDB) MemoryGetCollectionInfoWithType(collectionName string) map[string]interface{} {
	return d.MemoryGetCollectionInfo(collectionName)
}

// =============================================================================
// 文件路径辅助函数
// =============================================================================

// documentsFilePath 返回 text 文档分块文件路径
func (c *Collection) documentsFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("documents_%04d.json", chunkNum))
}

// imagesFilePath 返回 image 文档分块文件路径
func (c *Collection) imagesFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("images_%04d.json", chunkNum))
}

// tagsFilePath 返回标签向量分块文件路径
func (c *Collection) tagsFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("tags_%04d.json", chunkNum))
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

// atomicWriteJSON 原子写入 JSON 文件（先写临时文件，再重命名）
func atomicWriteJSON(path string, data interface{}) error {
	jsonBytes, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// =============================================================================
// 持久化 — 元数据、文档、标签向量
// =============================================================================

// saveCollectionMeta 保存集合元数据到 metadata.json
func (c *Collection) saveCollectionMeta() error {
	meta := collectionMeta{
		EmbeddingModel:      c.Model,
		EmbeddingDimension:  c.Dimension,
		MultimodalModel:     c.MultimodalModel,
		Type:                c.CollectionType,
		Version:             CurrentVersion,
		DocumentsChunkCount: c.documentsChunkCount,
		ImagesChunkCount:    c.imagesChunkCount,
		TagsChunkCount:      c.tagsChunkCount,
	}
	return atomicWriteJSON(c.metaPath, meta)
}

// updateLastModTime 更新最后修改时间戳
func (c *Collection) updateLastModTime() {
	if info, err := os.Stat(c.metaPath); err == nil {
		c.lastFileModTime = info.ModTime()
	}
}

// loadDocumentsFromFile 从分块文件加载文档到内存
func (c *Collection) loadDocumentsFromFile() error {
	var allDocs []Document

	if c.CollectionType == CollectionTypeImage {
		// 加载 images_*.json
		for i := 1; ; i++ {
			path := c.imagesFilePath(i)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				break
			}
			var docs []Document
			if err := readJSONFile(path, &docs); err != nil {
				return fmt.Errorf("读取 %s 失败: %w", path, err)
			}
			allDocs = append(allDocs, docs...)
			c.imagesChunkCount = i
		}
	} else {
		// 加载 documents_*.json
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
	}

	c.mu.Lock()
	c.Documents = allDocs
	c.mu.Unlock()

	return nil
}

// saveDocumentsToFile 将文档保存到分块文件
func (c *Collection) saveDocumentsToFile() error {
	c.mu.RLock()
	docs := make([]Document, len(c.Documents))
	copy(docs, c.Documents)
	c.mu.RUnlock()

	chunkSize := DocumentsChunkSize
	oldChunkCount := c.documentsChunkCount
	isImage := c.CollectionType == CollectionTypeImage

	if isImage {
		chunkSize = ImagesChunkSize
		oldChunkCount = c.imagesChunkCount
	}

	total := len(docs)
	newChunkCount := (total + chunkSize - 1) / chunkSize
	if total == 0 {
		newChunkCount = 0
	}

	// 写入各分块
	for i := 0; i < newChunkCount; i++ {
		chunkNum := i + 1
		start := i * chunkSize
		end := start + chunkSize
		if end > total {
			end = total
		}
		chunk := docs[start:end]

		var path string
		if isImage {
			path = c.imagesFilePath(chunkNum)
		} else {
			path = c.documentsFilePath(chunkNum)
		}

		if err := atomicWriteJSON(path, chunk); err != nil {
			return err
		}
	}

	// 清理多余的分块文件
	for i := newChunkCount + 1; i <= oldChunkCount; i++ {
		var path string
		if isImage {
			path = c.imagesFilePath(i)
		} else {
			path = c.documentsFilePath(i)
		}
		os.Remove(path)
	}

	if isImage {
		c.imagesChunkCount = newChunkCount
	} else {
		c.documentsChunkCount = newChunkCount
	}

	return c.saveCollectionMeta()
}

// loadTagsFromFile 从分块文件加载标签向量到内存
func (c *Collection) loadTagsFromFile() error {
	var meta collectionMeta
	if err := readJSONFile(c.metaPath, &meta); err != nil {
		return err
	}

	if meta.TagsChunkCount == 0 {
		c.mu.Lock()
		c.TagVectors = make([]TagVector, 0)
		c.mu.Unlock()
		c.tagsChunkCount = 0
		return nil
	}

	var allTags []TagVector
	for i := 1; i <= meta.TagsChunkCount; i++ {
		path := c.tagsFilePath(i)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		var tags []TagVector
		if err := readJSONFile(path, &tags); err != nil {
			return fmt.Errorf("读取 %s 失败: %w", path, err)
		}
		allTags = append(allTags, tags...)
	}

	c.mu.Lock()
	c.TagVectors = allTags
	c.mu.Unlock()
	c.tagsChunkCount = meta.TagsChunkCount

	return nil
}

// saveTagsToFile 将标签向量保存到分块文件
func (c *Collection) saveTagsToFile() error {
	c.mu.RLock()
	tags := make([]TagVector, len(c.TagVectors))
	copy(tags, c.TagVectors)
	c.mu.RUnlock()

	total := len(tags)
	newChunkCount := (total + TagsChunkSize - 1) / TagsChunkSize
	if total == 0 {
		newChunkCount = 0
	}

	// 写入各分块
	for i := 0; i < newChunkCount; i++ {
		chunkNum := i + 1
		start := i * TagsChunkSize
		end := start + TagsChunkSize
		if end > total {
			end = total
		}

		if err := atomicWriteJSON(c.tagsFilePath(chunkNum), tags[start:end]); err != nil {
			return err
		}
	}

	// 清理多余的分块文件
	for i := newChunkCount + 1; i <= c.tagsChunkCount; i++ {
		os.Remove(c.tagsFilePath(i))
	}

	c.tagsChunkCount = newChunkCount

	return c.saveCollectionMeta()
}

// reloadIfChanged 检测 metadata.json 是否被外部进程修改，若是则重新加载
func (c *Collection) reloadIfChanged() {
	if info, err := os.Stat(c.metaPath); err != nil {
		return
	} else if !info.ModTime().Equal(c.lastFileModTime) {
		c.loadDocumentsFromFile()
		c.loadTagsFromFile()
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
func CollectionInit(ctx context.Context, name, modelName, collectionType string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.CollectionInit(ctx, name, modelName, collectionType)
}

// MemoryAddMessage 全局添加消息
func MemoryAddMessage(ctx context.Context, collectionName, role, content string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessage(ctx, collectionName, role, content)
}

// MemoryAddMessageSilent 全局添加消息（无标签生成）
func MemoryAddMessageSilent(ctx context.Context, collectionName, role, content string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessageSilent(ctx, collectionName, role, content)
}

// MemoryAddImage 全局添加图片
func MemoryAddImage(ctx context.Context, collectionName, base64Image string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddImage(ctx, collectionName, base64Image)
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

// =============================================================================
// 调试接口 — 供测试模块使用
// =============================================================================

// MemoryDebugGetRawTags 获取指定集合的原始标签向量数据（调试用）
func MemoryDebugGetRawTags(collectionName string) interface{} {
	if globalMemoryDB == nil {
		return nil
	}
	c, err := globalMemoryDB.getCollection(collectionName)
	if err != nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.TagVectors
}
