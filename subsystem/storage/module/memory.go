package module

import (
	"config"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"logger"
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
// 使用 crypto/rand 提供密码学级随机源，比 math/rand 更不可预测
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read 失败极少见；退化为时间戳种子避免阻塞调用方
		logger.Warn("Storage", "crypto/rand 失败, 退化为时间戳 UUID: %v", err)
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (i % 8 * 8))
		}
	}
	// 版本位：高 4 位固定为 0100（版本 4）
	b[6] = (b[6] & 0x0f) | 0x40
	// 变体位：高 2 位固定为 10（RFC 4122 变体）
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// =============================================================================
// 记忆库初始化
// =============================================================================

// InitMemoryDB 初始化记忆库存储根目录
// 创建 baseDir、迁移旧 collections/ 层级、初始化集合 map，并赋值给全局 MemoryDatabase 实例
// 此函数不产生网络请求，仅准备本地存储结构；嵌入服务连接由 MemoryInitInstance 配置
func InitMemoryDB(memoryDir string) error {
	if MemoryDatabase != nil && MemoryDatabase.memoryInitialized {
		return nil
	}

	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return fmt.Errorf("创建记忆库目录失败: %v", err)
	}

	db := &MemoryDB{
		baseDir:     memoryDir,
		collections: make(map[string]*Collection),
	}
	MemoryDatabase = db

	logger.Info("Storage", "记忆库存储目录已就绪: %s", memoryDir)
	return nil
}

// MemoryInitInstance 初始化记忆库实例（不创建任何集合）
// 仅配置嵌入服务连接，并加载已存在的集合到内存
// 方法接收者为 *MemoryDB；全局包装函数同名 MemoryInitInstance 负责实例懒初始化
func (d *MemoryDB) MemoryInitInstance(baseURL string, apiKey string) error {
	if d.memoryInitialized {
		return nil
	}

	if d.baseDir == "" {
		return fmt.Errorf("记忆库未配置存储路径, 请先调用 InitMemoryDB")
	}

	d.embeddingBaseURL = baseURL
	d.embeddingAPIKey = apiKey
	d.httpClient = &http.Client{Timeout: 120 * time.Second}
	d.memoryInitialized = true

	d.loadAllCollections()

	logger.Info("Storage", "记忆库实例初始化完成, base_url: %s, 已加载 %d 个集合",
		d.embeddingBaseURL, len(d.collections))
	return nil
}

// validateCollectionName 校验集合名合法性（仅字母数字下划线连字符，防路径穿越）
// 同时拒绝 URL 路由保留名（init/stats/collections），避免与端点路径冲突
func validateCollectionName(name string) error {
	if name == "" {
		return fmt.Errorf("集合名不能为空")
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("集合名仅允许字母、数字、下划线、连字符: %s", name)
		}
	}
	// 拒绝 URL 路由保留名，避免与 /memory/init、/memory/stats、/memory/collections 冲突
	switch name {
	case "init", "stats", "collections":
		return fmt.Errorf("集合名不能使用保留字: %s", name)
	}
	return nil
}

// CollectionInit 创建或打开指定名称的集合（默认 text 类型）
// 通过探针文本嵌入一次确定向量维度，写入 metadata.json
// 若集合已存在且 model 一致则直接返回，model 变更则重新探针并更新维度
func (d *MemoryDB) CollectionInit(ctx context.Context, name string, modelName string) error {
	return d.collectionInit(ctx, name, modelName, CollectionTypeText)
}

// CollectionInitImage 创建或打开指定名称的 image 类型集合
// image 集合使用三元嵌入向量（情绪 + 色彩风格 + 主要内容），存储 base64 图片数据
// 通过探针文本嵌入一次确定向量维度，写入 metadata.json（含 type="image"）
func (d *MemoryDB) CollectionInitImage(ctx context.Context, name string, modelName string) error {
	return d.collectionInit(ctx, name, modelName, CollectionTypeImage)
}

// collectionInit 内部通用集合初始化逻辑
func (d *MemoryDB) collectionInit(ctx context.Context, name string, modelName string, collType string) error {
	if !d.memoryInitialized {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	if err := validateCollectionName(name); err != nil {
		return err
	}

	// 已存在则直接返回
	d.collectionsMu.RLock()
	if existing, ok := d.collections[name]; ok {
		// 已存在集合的类型必须与请求一致
		if existing.CollectionType != collType {
			d.collectionsMu.RUnlock()
			return fmt.Errorf("集合 [%s] 已存在且类型为 %s, 无法以 %s 类型重新初始化", name, existing.CollectionType, collType)
		}
		d.collectionsMu.RUnlock()
		return nil
	}
	d.collectionsMu.RUnlock()

	// 扁平化存储：<baseDir>/<name>/
	collDir := filepath.Join(d.baseDir, name)
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return fmt.Errorf("创建集合目录失败: %v", err)
	}

	metaPath := filepath.Join(collDir, "metadata.json")

	// 尝试加载已有 metadata
	var meta collectionMeta
	if data, err := os.ReadFile(metaPath); err == nil && len(data) > 0 {
		if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
			return fmt.Errorf("metadata.json 解析失败: %v", jsonErr)
		}
	}

	// 若已有 metadata 且类型字段存在，以既有类型为准；否则写入请求类型
	if meta.Type == "" {
		meta.Type = collType
	} else if meta.Type != collType {
		return fmt.Errorf("集合 [%s] 磁盘 metadata 类型为 %s, 与请求类型 %s 不一致", name, meta.Type, collType)
	}

	// metadata 不存在或 model 变更时，重新探针定维度
	if meta.Dimension == 0 || meta.Model != modelName {
		probeVec, err := d.embedText(ctx, modelName, name)
		if err != nil {
			return fmt.Errorf("探针文本嵌入失败: %v", err)
		}
		meta.Model = modelName
		meta.Dimension = len(probeVec)
		if err := atomicWriteJSON(metaPath, meta); err != nil {
			return fmt.Errorf("写入 metadata.json 失败: %v", err)
		}
	}

	c := &Collection{
		Name:           name,
		Model:          meta.Model,
		Dimension:      meta.Dimension,
		CollectionType: meta.Type,
		Documents:      make([]MemoryDocument, 0),
		ImageDocuments: make([]ImageDocument, 0),
		collDir:        collDir,
		metaPath:       metaPath,
	}
	c.loadDocumentsFromFile()

	d.collectionsMu.Lock()
	d.collections[name] = c
	d.collectionsMu.Unlock()

	logger.Info("Storage", "集合 [%s] 初始化完成, 类型: %s, 模型: %s, 维度: %d, 文档数: %d",
		name, c.CollectionType, c.Model, c.Dimension, c.docCount())
	return nil
}

// getCollection 获取集合实例，不存在返回错误
func (d *MemoryDB) getCollection(name string) (*Collection, error) {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	c, ok := d.collections[name]
	if !ok {
		return nil, fmt.Errorf("集合 [%s] 不存在, 请先调用 CollectionInit", name)
	}
	return c, nil
}

// loadAllCollections 启动时扫描 baseDir 加载所有集合到内存
// 扁平化布局下，baseDir 的每个子目录即为一个集合
// 根据 metadata.json 中的 type 字段区分 text/image 集合，分别加载对应文档
func (d *MemoryDB) loadAllCollections() {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "扫描记忆存储目录失败: %v", err)
		}
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if validateCollectionName(name) != nil {
			continue
		}
		collDir := filepath.Join(d.baseDir, name)
		metaPath := filepath.Join(collDir, "metadata.json")

		var meta collectionMeta
		if data, err := os.ReadFile(metaPath); err == nil && len(data) > 0 {
			if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
				logger.Warn("Storage", "集合 [%s] metadata 解析失败: %v", name, jsonErr)
				continue
			}
		}

		if meta.Model == "" || meta.Dimension == 0 {
			logger.Warn("Storage", "集合 [%s] metadata 不完整, 跳过加载", name)
			continue
		}

		// 未指定类型时默认为 text
		collType := meta.Type
		if collType == "" {
			collType = CollectionTypeText
		}

		c := &Collection{
			Name:           name,
			Model:          meta.Model,
			Dimension:      meta.Dimension,
			CollectionType: collType,
			Documents:      make([]MemoryDocument, 0),
			ImageDocuments: make([]ImageDocument, 0),
			collDir:        collDir,
			metaPath:       metaPath,
		}
		c.loadDocumentsFromFile()

		d.collectionsMu.Lock()
		d.collections[name] = c
		d.collectionsMu.Unlock()

		logger.Info("Storage", "已加载集合 [%s], 类型: %s, 模型: %s, 维度: %d, 文档数: %d",
			name, c.CollectionType, c.Model, c.Dimension, c.docCount())
	}
}

// IsMemoryInitialized 返回记忆库实例是否已初始化
func (d *MemoryDB) IsMemoryInitialized() bool {
	return d != nil && d.memoryInitialized
}

// docCount 返回集合中文档总数（根据类型返回对应文档数）
func (c *Collection) docCount() int {
	if c.CollectionType == CollectionTypeImage {
		return len(c.ImageDocuments)
	}
	return len(c.Documents)
}

// =============================================================================
// image 集合专用文件路径方法
// =============================================================================

// base64FilePath 返回指定分块编号的 base64 图片数据文件路径
// 格式：<collDir>/base64_0001.json（仅 image 类型集合使用）
func (c *Collection) base64FilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("base64_%04d.json", chunkNum))
}

// =============================================================================
// 记忆库操作（多集合）
// =============================================================================

// MemoryAddMessage 向指定集合添加一条消息，返回新生成的 UUID 文档 ID
// ID 采用 UUID v4 格式；旧版 msg-N 格式 ID 仅在历史数据加载时保留，新增文档一律使用 UUID
func (d *MemoryDB) MemoryAddMessage(ctx context.Context, collectionName string, role string, content string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("消息内容不能为空")
	}

	embedding, err := d.embedText(ctx, c.Model, content)
	if err != nil {
		return "", fmt.Errorf("嵌入文本失败: %v", err)
	}

	if len(embedding) != c.Dimension {
		return "", fmt.Errorf("嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(embedding), collectionName, c.Dimension)
	}

	id := generateUUID()
	c.mu.Lock()
	c.Documents = append(c.Documents, MemoryDocument{
		ID:        id,
		Role:      role,
		Content:   content,
		Embedding: embedding,
	})
	c.mu.Unlock()

	c.saveDocumentsToFile()
	return id, nil
}

// MemoryAddMessageSilent 添加消息但不返回 ID，仅返回错误
func (d *MemoryDB) MemoryAddMessageSilent(ctx context.Context, collectionName string, role string, content string) error {
	_, err := d.MemoryAddMessage(ctx, collectionName, role, content)
	return err
}

// MemoryQueryMessages 按查询文本检索最相似的消息，返回记忆消息兼容格式的 JSON 字符串列表
func (d *MemoryDB) MemoryQueryMessages(ctx context.Context, collectionName string, queryText string, topK int) ([]string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	if topK <= 0 {
		topK = 10
	}

	queryVec, err := d.embedText(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("嵌入查询文本失败: %v", err)
	}

	if len(queryVec) != c.Dimension {
		return nil, fmt.Errorf("查询嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(queryVec), collectionName, c.Dimension)
	}

	results := c.queryTopK(queryVec, topK)

	messages := make([]string, 0, len(results))
	for _, r := range results {
		msg := memoryMessage{Role: r.Role, Content: r.Content}
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		messages = append(messages, string(jsonBytes))
	}
	return messages, nil
}

// MemoryQueryMessagesWithContent 按查询文本检索最相似的消息，返回含 ID/角色/内容/相似度的结构化结果
func (d *MemoryDB) MemoryQueryMessagesWithContent(ctx context.Context, collectionName string, queryText string, topK int) ([]MemoryQueryResult, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	if topK <= 0 {
		topK = 10
	}

	queryVec, err := d.embedText(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("嵌入查询文本失败: %v", err)
	}

	if len(queryVec) != c.Dimension {
		return nil, fmt.Errorf("查询嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(queryVec), collectionName, c.Dimension)
	}

	return c.queryTopK(queryVec, topK), nil
}

// MemoryDeleteMessage 按 ID 删除指定集合中的一条文档
// 兼容 UUID 与旧版 msg-N 两种 ID 格式（按字符串相等匹配）
// 根据集合类型分别操作 Documents 或 ImageDocuments
func (d *MemoryDB) MemoryDeleteMessage(ctx context.Context, collectionName string, id string) error {
	_ = ctx
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	if c.CollectionType == CollectionTypeImage {
		c.mu.Lock()
		for i, doc := range c.ImageDocuments {
			if doc.ID == id {
				c.ImageDocuments = append(c.ImageDocuments[:i], c.ImageDocuments[i+1:]...)
				c.mu.Unlock()
				c.saveDocumentsToFile()
				return nil
			}
		}
		c.mu.Unlock()
		return nil
	}

	c.mu.Lock()
	for i, doc := range c.Documents {
		if doc.ID == id {
			c.Documents = append(c.Documents[:i], c.Documents[i+1:]...)
			c.mu.Unlock()
			c.saveDocumentsToFile()
			return nil
		}
	}
	c.mu.Unlock()
	return nil
}

// MemoryGetCollectionCount 返回集合中文档总数（根据类型返回对应文档数）
func (d *MemoryDB) MemoryGetCollectionCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.reloadIfChanged()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.docCount()
}

// MemoryGetDocuments 分页返回集合文档条目（不含嵌入向量），同时返回总数
// image 类型集合返回的 DocumentEntry 中，Content 字段存储图片 ID（base64 数据不在此返回）
func (d *MemoryDB) MemoryGetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return []DocumentEntry{}, 0
	}

	// 跨进程一致性检测：若磁盘文件被其他进程更新则重载内存缓存
	c.reloadIfChanged()

	if c.CollectionType == CollectionTypeImage {
		c.mu.RLock()
		defer c.mu.RUnlock()

		total := len(c.ImageDocuments)
		if offset < 0 {
			offset = 0
		}
		if offset >= total {
			return []DocumentEntry{}, total
		}

		end := offset + limit
		if end > total {
			end = total
		}

		entries := make([]DocumentEntry, end-offset)
		for i := offset; i < end; i++ {
			entries[i-offset] = DocumentEntry{
				ID:      c.ImageDocuments[i].ID,
				Role:    "image",
				Content: c.ImageDocuments[i].ID,
			}
		}
		return entries, total
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	total := len(c.Documents)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []DocumentEntry{}, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	entries := make([]DocumentEntry, end-offset)
	for i := offset; i < end; i++ {
		entries[i-offset] = DocumentEntry{
			ID:      c.Documents[i].ID,
			Role:    c.Documents[i].Role,
			Content: c.Documents[i].Content,
		}
	}
	return entries, total
}

// MemoryGetEntryCount 返回集合中文档总数（与 MemoryGetCollectionCount 等价，语义别名）
func (d *MemoryDB) MemoryGetEntryCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.reloadIfChanged()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.docCount()
}

// MemoryHasSyncMismatch 检测集合内是否有文档向量缺失或维度与集合锁定维度不符
// image 类型集合检查三元嵌入向量中每个向量的维度
func (d *MemoryDB) MemoryHasSyncMismatch(collectionName string) bool {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.CollectionType == CollectionTypeImage {
		for _, doc := range c.ImageDocuments {
			for v := 0; v < 3; v++ {
				if len(doc.Embeddings[v]) != c.Dimension {
					return true
				}
			}
		}
		return false
	}

	for _, doc := range c.Documents {
		if len(doc.Embedding) != c.Dimension {
			return true
		}
	}
	return false
}

// MemoryDeleteCollection 删除整个集合：从内存中移除，删除磁盘目录
// 操作不可恢复，调用方应确保前端已做二次确认
func (d *MemoryDB) MemoryDeleteCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	collDir := c.collDir
	docCount := c.docCount()

	d.collectionsMu.Lock()
	delete(d.collections, collectionName)
	d.collectionsMu.Unlock()

	if err := os.RemoveAll(collDir); err != nil {
		return fmt.Errorf("删除集合目录失败: %v", err)
	}

	logger.Info("Storage", "集合 [%s] 已删除, 类型: %s, 文档数: %d, 目录: %s",
		collectionName, c.CollectionType, docCount, collDir)
	return nil
}

// MemoryClearCollection 清空集合中所有文档（保留集合元数据，仅删除文档）
// 操作不可恢复，调用方应确保前端已做二次确认
// 根据集合类型清空对应的文档列表
func (d *MemoryDB) MemoryClearCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	var originalCount int

	if c.CollectionType == CollectionTypeImage {
		c.mu.Lock()
		originalCount = len(c.ImageDocuments)
		c.ImageDocuments = make([]ImageDocument, 0)
		c.mu.Unlock()
	} else {
		c.mu.Lock()
		originalCount = len(c.Documents)
		c.Documents = make([]MemoryDocument, 0)
		c.mu.Unlock()
	}

	c.saveDocumentsToFile()

	logger.Info("Storage", "集合 [%s] 已清空, 类型: %s, 删除文档数: %d",
		collectionName, c.CollectionType, originalCount)
	return nil
}

// MemoryRebuildEntries 删除向量缺失或维度不符的文档，重新持久化
// ctx 保留以兼容签名，当前实现不调用嵌入服务
// image 类型集合检查三元嵌入向量中每个向量的维度
func (d *MemoryDB) MemoryRebuildEntries(ctx context.Context, collectionName string) (int, error) {
	_ = ctx
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0, err
	}

	if c.CollectionType == CollectionTypeImage {
		c.mu.Lock()
		original := len(c.ImageDocuments)
		filtered := make([]ImageDocument, 0, original)
		removed := 0
		for _, doc := range c.ImageDocuments {
			dimOk := true
			for v := 0; v < 3; v++ {
				if len(doc.Embeddings[v]) != c.Dimension {
					dimOk = false
					break
				}
			}
			if !dimOk {
				removed++
				continue
			}
			filtered = append(filtered, doc)
		}
		c.ImageDocuments = filtered
		c.mu.Unlock()

		if removed > 0 {
			c.saveDocumentsToFile()
			logger.Info("Storage", "图片集合 [%s] 重建完成, 原始 %d 条, 删除 %d 条维度不符, 剩余 %d 条",
				collectionName, original, removed, len(filtered))
		} else {
			logger.Info("Storage", "图片集合 [%s] 重建完成, 无异常文档, 共 %d 条", collectionName, original)
		}
		return len(filtered), nil
	}

	c.mu.Lock()
	original := len(c.Documents)
	filtered := make([]MemoryDocument, 0, original)
	removed := 0
	for _, doc := range c.Documents {
		if len(doc.Embedding) != c.Dimension {
			removed++
			continue
		}
		filtered = append(filtered, doc)
	}
	c.Documents = filtered
	c.mu.Unlock()

	if removed > 0 {
		c.saveDocumentsToFile()
		logger.Info("Storage", "集合 [%s] 重建完成, 原始 %d 条, 删除 %d 条维度不符, 剩余 %d 条",
			collectionName, original, removed, len(filtered))
	} else {
		logger.Info("Storage", "集合 [%s] 重建完成, 无异常文档, 共 %d 条", collectionName, original)
	}

	return len(filtered), nil
}

// MemoryListCollections 返回所有已加载集合的名称
func (d *MemoryDB) MemoryListCollections() []string {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	names := make([]string, 0, len(d.collections))
	for name := range d.collections {
		names = append(names, name)
	}
	return names
}

// MemoryGetCollectionInfo 返回集合元信息（模型、维度、文档数）
func (d *MemoryDB) MemoryGetCollectionInfo(collectionName string) (model string, dimension int, count int, err error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", 0, 0, err
	}
	c.reloadIfChanged()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Model, c.Dimension, c.docCount(), nil
}

// MemoryGetCollectionInfoWithType 返回集合元信息（模型、维度、文档数、类型）
func (d *MemoryDB) MemoryGetCollectionInfoWithType(collectionName string) (model string, dimension int, count int, collType string) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", 0, 0, ""
	}
	c.reloadIfChanged()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Model, c.Dimension, c.docCount(), c.CollectionType
}

// =============================================================================
// Collection 持久化方法 — 分块存储（contents_NNNN.json + embeddings_NNNN.json）
// =============================================================================

// contentFilePath 返回指定分块编号的内容文件路径
// 格式：<collDir>/contents_0001.json
func (c *Collection) contentFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("contents_%04d.json", chunkNum))
}

// embeddingFilePath 返回指定分块编号的嵌入向量文件路径
// 格式：<collDir>/embeddings_0001.json
func (c *Collection) embeddingFilePath(chunkNum int) string {
	return filepath.Join(c.collDir, fmt.Sprintf("embeddings_%04d.json", chunkNum))
}

// atomicWriteJSON 原子化写入 JSON 文件：写临时文件 + delete + rename
// Windows 兼容：rename 不能覆盖已存在文件，需先删除
func atomicWriteJSON(path string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("临时文件写入失败: %w", err)
	}
	os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("原子重命名失败: %w", err)
	}
	return nil
}

// readJSONFile 读取并反序列化 JSON 文件到指定目标
func readJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// saveCollectionMeta 原子化写入集合元数据（含分块计数与集合类型），并更新 lastFileModTime
// 在所有分块文件写入完成后调用，确保跨进程读取一致性
func (c *Collection) saveCollectionMeta() error {
	meta := collectionMeta{
		Model:      c.Model,
		Dimension:  c.Dimension,
		ChunkCount: c.chunkCount,
		Type:       c.CollectionType,
	}
	if err := atomicWriteJSON(c.metaPath, meta); err != nil {
		return err
	}
	// 更新本进程记录的文件修改时间，避免 reloadIfChanged 误判本进程写入为外部更新
	if fi, statErr := os.Stat(c.metaPath); statErr == nil {
		c.mu.Lock()
		c.lastFileModTime = fi.ModTime()
		c.mu.Unlock()
	}
	return nil
}

// updateLastModTime 更新 lastFileModTime 为 metadata.json 的当前修改时间
func (c *Collection) updateLastModTime() {
	if fi, err := os.Stat(c.metaPath); err == nil {
		c.mu.Lock()
		c.lastFileModTime = fi.ModTime()
		c.mu.Unlock()
	}
}

// loadDocumentsFromFile 加载文档到集合内存
// 根据 CollectionType 分派到对应加载逻辑：
//   - text 类型：自动检测旧格式（documents.json）并即时迁移到新格式（分块存储）
//   - image 类型：从 base64 分块文件 + embeddings 分块文件加载
//
// 兼容旧版 msg-N 格式 ID 与新版 UUID 格式 ID，加载时保留原值不做改写
func (c *Collection) loadDocumentsFromFile() {
	if c.CollectionType == CollectionTypeImage {
		c.loadImageDocumentsFromFile()
		c.updateLastModTime()
		return
	}

	oldPath := filepath.Join(c.collDir, "documents.json")

	if _, err := os.Stat(oldPath); err == nil {
		// 旧格式分支：加载 documents.json，然后立即迁移到新格式
		c.loadFromOldFormat(oldPath)
		if err := c.migrateToNewFormat(); err != nil {
			logger.Error("Storage", "集合 [%s] 迁移到新格式失败: %v, 保留旧文件", c.Name, err)
			return
		}
		// 迁移成功后删除旧文件
		if err := os.Remove(oldPath); err != nil {
			logger.Warn("Storage", "集合 [%s] 删除旧 documents.json 失败: %v", c.Name, err)
		}
		logger.Info("Storage", "集合 [%s] 已从旧格式迁移到分块存储, 分块数: %d, 文档数: %d",
			c.Name, c.chunkCount, len(c.Documents))
	} else {
		// 新格式分支：从分块文件加载
		c.loadFromChunks()
	}

	// 统一更新 lastFileModTime 为 metadata.json 的修改时间
	c.updateLastModTime()
}

// loadFromOldFormat 从旧格式 documents.json 加载文档到内存
func (c *Collection) loadFromOldFormat(oldPath string) {
	data, err := os.ReadFile(oldPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "集合 [%s] 读取 documents.json 失败: %v", c.Name, err)
		}
		return
	}
	if len(data) == 0 {
		c.mu.Lock()
		c.Documents = make([]MemoryDocument, 0)
		c.mu.Unlock()
		return
	}
	var docs []MemoryDocument
	if err := json.Unmarshal(data, &docs); err != nil {
		logger.Warn("Storage", "集合 [%s] documents.json 解析失败: %v", c.Name, err)
		return
	}
	c.mu.Lock()
	c.Documents = docs
	c.mu.Unlock()
}

// loadFromChunks 从分块文件加载文档到内存
// 按 metadata.json 中的 chunk_count 遍历加载所有内容与嵌入向量分块，按 ID 合并
func (c *Collection) loadFromChunks() {
	// 读取 metadata.json 获取分块数
	var meta collectionMeta
	if err := readJSONFile(c.metaPath, &meta); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "集合 [%s] 读取 metadata.json 失败: %v", c.Name, err)
		}
		return
	}

	if meta.ChunkCount == 0 {
		c.mu.Lock()
		c.Documents = make([]MemoryDocument, 0)
		c.mu.Unlock()
		c.chunkCount = 0
		return
	}

	// 加载所有内容分块和嵌入向量分块
	allContents := make([]contentEntry, 0)
	embedMap := make(map[string][]float32)

	for i := 1; i <= meta.ChunkCount; i++ {
		// 加载内容分块
		var contents []contentEntry
		if err := readJSONFile(c.contentFilePath(i), &contents); err != nil {
			logger.Warn("Storage", "集合 [%s] 读取 contents_%04d.json 失败: %v, 跳过该分块", c.Name, i, err)
			continue
		}
		allContents = append(allContents, contents...)

		// 加载嵌入向量分块
		var embeddings []embeddingEntry
		if err := readJSONFile(c.embeddingFilePath(i), &embeddings); err != nil {
			logger.Warn("Storage", "集合 [%s] 读取 embeddings_%04d.json 失败: %v, 跳过该分块", c.Name, i, err)
			continue
		}
		for _, e := range embeddings {
			embedMap[e.ID] = e.Embedding
		}
	}

	// 按 ID 合并内容与嵌入向量
	docs := make([]MemoryDocument, 0, len(allContents))
	for _, ct := range allContents {
		emb, ok := embedMap[ct.ID]
		if !ok {
			logger.Warn("Storage", "集合 [%s] 文档 %s 缺少嵌入向量 (文件可能损坏), 跳过", c.Name, ct.ID)
			continue
		}
		docs = append(docs, MemoryDocument{
			ID:        ct.ID,
			Role:      ct.Role,
			Content:   ct.Content,
			Embedding: emb,
		})
	}

	c.mu.Lock()
	c.Documents = docs
	c.mu.Unlock()
	c.chunkCount = meta.ChunkCount
}

// migrateToNewFormat 将内存中的文档写入新格式分块文件
// 调用方必须确保 c.Documents 已经加载完毕
func (c *Collection) migrateToNewFormat() error {
	c.mu.RLock()
	totalDocs := len(c.Documents)
	c.mu.RUnlock()

	newChunkCount := (totalDocs + MemoryChunkSize - 1) / MemoryChunkSize

	// 写入所有分块
	for i := 0; i < newChunkCount; i++ {
		chunkNum := i + 1
		start := i * MemoryChunkSize
		end := start + MemoryChunkSize
		if end > totalDocs {
			end = totalDocs
		}

		c.mu.RLock()
		chunkDocs := c.Documents[start:end]
		c.mu.RUnlock()

		contents := make([]contentEntry, len(chunkDocs))
		embeddings := make([]embeddingEntry, len(chunkDocs))
		for j, doc := range chunkDocs {
			contents[j] = contentEntry{ID: doc.ID, Role: doc.Role, Content: doc.Content}
			embeddings[j] = embeddingEntry{ID: doc.ID, Embedding: doc.Embedding}
		}

		if err := atomicWriteJSON(c.contentFilePath(chunkNum), contents); err != nil {
			return fmt.Errorf("写入 contents_%04d.json 失败: %w", chunkNum, err)
		}
		if err := atomicWriteJSON(c.embeddingFilePath(chunkNum), embeddings); err != nil {
			return fmt.Errorf("写入 embeddings_%04d.json 失败: %w", chunkNum, err)
		}
	}

	c.chunkCount = newChunkCount

	// 更新 metadata.json（含 chunk_count）
	if err := c.saveCollectionMeta(); err != nil {
		return fmt.Errorf("更新 metadata.json 失败: %w", err)
	}

	return nil
}

// reloadIfChanged 跨进程一致性检测：若 metadata.json 被其他进程修改则重载到内存
// 场景：crystal_astral（前端 HTTP 服务）与 lunar_astral（AI 引擎）为两个独立进程，
// 共享同一磁盘文件但各自维护独立内存缓存。lunar_astral 写入后，crystal_astral 的
// 内存缓存会过期；此方法在读路径调用前比对 metadata.json 修改时间，发现变化即重载。
// 新格式下以 metadata.json 为检测锚点：每次写入都会在所有分块完成后更新 metadata.json，
// 保证跨进程读取到一致的数据。
func (c *Collection) reloadIfChanged() {
	fi, err := os.Stat(c.metaPath)
	if err != nil {
		return
	}
	c.mu.RLock()
	last := c.lastFileModTime
	c.mu.RUnlock()
	if !fi.ModTime().After(last) {
		return
	}
	logger.Info("Storage", "集合 [%s] 检测到 metadata.json 被外部更新, 重新加载 (旧 mtime: %v, 新 mtime: %v)",
		c.Name, last, fi.ModTime())
	c.loadDocumentsFromFile()
}

// saveDocumentsToFile 将内存中的文档写入分块存储文件
// 根据 CollectionType 分派到对应保存逻辑：
//   - text 类型：按 MemoryChunkSize（100 条）切分文档，写入 contents_NNNN.json + embeddings_NNNN.json
//   - image 类型：按 MemoryChunkSize（100 条）切分文档，写入 base64_NNNN.json + embeddings_NNNN.json
//
// 自动清理多余分块文件（当文档总数减少时），最后原子化更新 metadata.json
func (c *Collection) saveDocumentsToFile() {
	if c.CollectionType == CollectionTypeImage {
		c.saveImageDocumentsToFile()
		return
	}

	c.mu.RLock()
	totalDocs := len(c.Documents)
	// 浅拷贝文档切片，避免长时间持锁
	docs := make([]MemoryDocument, totalDocs)
	copy(docs, c.Documents)
	c.mu.RUnlock()

	newChunkCount := (totalDocs + MemoryChunkSize - 1) / MemoryChunkSize

	// 写入所有新分块
	for i := 0; i < newChunkCount; i++ {
		chunkNum := i + 1
		start := i * MemoryChunkSize
		end := start + MemoryChunkSize
		if end > totalDocs {
			end = totalDocs
		}

		chunkDocs := docs[start:end]
		contents := make([]contentEntry, len(chunkDocs))
		embeddings := make([]embeddingEntry, len(chunkDocs))
		for j, doc := range chunkDocs {
			contents[j] = contentEntry{ID: doc.ID, Role: doc.Role, Content: doc.Content}
			embeddings[j] = embeddingEntry{ID: doc.ID, Embedding: doc.Embedding}
		}

		if err := atomicWriteJSON(c.contentFilePath(chunkNum), contents); err != nil {
			logger.Error("Storage", "集合 [%s] contents_%04d.json 写入失败: %v", c.Name, chunkNum, err)
			return
		}
		if err := atomicWriteJSON(c.embeddingFilePath(chunkNum), embeddings); err != nil {
			logger.Error("Storage", "集合 [%s] embeddings_%04d.json 写入失败: %v", c.Name, chunkNum, err)
			return
		}
	}

	// 删除多余的分块文件（当文档总数减少导致分块数下降时）
	oldChunkCount := c.chunkCount
	for i := newChunkCount + 1; i <= oldChunkCount; i++ {
		os.Remove(c.contentFilePath(i))
		os.Remove(c.embeddingFilePath(i))
	}

	c.chunkCount = newChunkCount

	// 原子化更新 metadata.json
	if err := c.saveCollectionMeta(); err != nil {
		logger.Error("Storage", "集合 [%s] metadata.json 更新失败: %v", c.Name, err)
	}

	// 清理可能残留的旧格式 documents.json（迁移后遗留或异常情况）
	oldPath := filepath.Join(c.collDir, "documents.json")
	os.Remove(oldPath) // 忽略错误，文件可能不存在
}

// =============================================================================
// image 集合持久化方法 — 分块存储（base64_NNNN.json + embeddings_NNNN.json）
// =============================================================================

// loadImageDocumentsFromFile 从 image 类型分块文件加载图片文档到集合内存
// 按 metadata.json 中的 chunk_count 遍历加载所有 base64 与 embeddings 分块，按 ID 合并
func (c *Collection) loadImageDocumentsFromFile() {
	// 读取 metadata.json 获取分块数
	var meta collectionMeta
	if err := readJSONFile(c.metaPath, &meta); err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "集合 [%s] 读取 metadata.json 失败: %v", c.Name, err)
		}
		return
	}

	if meta.ChunkCount == 0 {
		c.mu.Lock()
		c.ImageDocuments = make([]ImageDocument, 0)
		c.mu.Unlock()
		c.chunkCount = 0
		return
	}

	// 加载所有 base64 分块和 embeddings 分块
	allBase64 := make([]base64Entry, 0)
	embedMap := make(map[string][3][]float32)

	for i := 1; i <= meta.ChunkCount; i++ {
		// 加载 base64 分块
		var base64s []base64Entry
		if err := readJSONFile(c.base64FilePath(i), &base64s); err != nil {
			logger.Warn("Storage", "集合 [%s] 读取 base64_%04d.json 失败: %v, 跳过该分块", c.Name, i, err)
			continue
		}
		allBase64 = append(allBase64, base64s...)

		// 加载 embeddings 分块（image 格式：三元嵌入向量）
		var embeddings []imageEmbeddingEntry
		if err := readJSONFile(c.embeddingFilePath(i), &embeddings); err != nil {
			logger.Warn("Storage", "集合 [%s] 读取 embeddings_%04d.json 失败: %v, 跳过该分块", c.Name, i, err)
			continue
		}
		for _, e := range embeddings {
			embedMap[e.ID] = e.Embeddings
		}
	}

	// 按 ID 合并 base64 数据与三元嵌入向量
	docs := make([]ImageDocument, 0, len(allBase64))
	for _, b64 := range allBase64 {
		emb, ok := embedMap[b64.ID]
		if !ok {
			logger.Warn("Storage", "集合 [%s] 图片文档 %s 缺少嵌入向量 (文件可能损坏), 跳过", c.Name, b64.ID)
			continue
		}
		// 验证三个嵌入向量维度均与集合锁定维度一致
		dimOk := true
		for idx := range emb {
			if len(emb[idx]) != c.Dimension {
				logger.Warn("Storage", "集合 [%s] 图片文档 %s 嵌入向量[%d] 维度 %d 与集合维度 %d 不符, 跳过",
					c.Name, b64.ID, idx, len(emb[idx]), c.Dimension)
				dimOk = false
				break
			}
		}
		if !dimOk {
			continue
		}
		docs = append(docs, ImageDocument{
			ID:         b64.ID,
			Image:      b64.Image,
			Embeddings: emb,
		})
	}

	c.mu.Lock()
	c.ImageDocuments = docs
	c.mu.Unlock()
	c.chunkCount = meta.ChunkCount
}

// saveImageDocumentsToFile 将内存中的图片文档写入分块存储文件
// 按 MemoryChunkSize（100 条）切分文档，写入 base64_NNNN.json + embeddings_NNNN.json
// 自动清理多余分块文件，最后原子化更新 metadata.json
func (c *Collection) saveImageDocumentsToFile() {
	c.mu.RLock()
	totalDocs := len(c.ImageDocuments)
	// 浅拷贝文档切片，避免长时间持锁
	docs := make([]ImageDocument, totalDocs)
	copy(docs, c.ImageDocuments)
	c.mu.RUnlock()

	newChunkCount := (totalDocs + MemoryChunkSize - 1) / MemoryChunkSize

	// 写入所有新分块
	for i := 0; i < newChunkCount; i++ {
		chunkNum := i + 1
		start := i * MemoryChunkSize
		end := start + MemoryChunkSize
		if end > totalDocs {
			end = totalDocs
		}

		chunkDocs := docs[start:end]
		base64s := make([]base64Entry, len(chunkDocs))
		embeddings := make([]imageEmbeddingEntry, len(chunkDocs))
		for j, doc := range chunkDocs {
			base64s[j] = base64Entry{ID: doc.ID, Image: doc.Image}
			embeddings[j] = imageEmbeddingEntry{ID: doc.ID, Embeddings: doc.Embeddings}
		}

		if err := atomicWriteJSON(c.base64FilePath(chunkNum), base64s); err != nil {
			logger.Error("Storage", "集合 [%s] base64_%04d.json 写入失败: %v", c.Name, chunkNum, err)
			return
		}
		if err := atomicWriteJSON(c.embeddingFilePath(chunkNum), embeddings); err != nil {
			logger.Error("Storage", "集合 [%s] embeddings_%04d.json 写入失败: %v", c.Name, chunkNum, err)
			return
		}
	}

	// 删除多余的分块文件
	oldChunkCount := c.chunkCount
	for i := newChunkCount + 1; i <= oldChunkCount; i++ {
		os.Remove(c.base64FilePath(i))
		os.Remove(c.embeddingFilePath(i))
	}

	c.chunkCount = newChunkCount

	// 原子化更新 metadata.json
	if err := c.saveCollectionMeta(); err != nil {
		logger.Error("Storage", "集合 [%s] metadata.json 更新失败: %v", c.Name, err)
	}
}

// =============================================================================
// image 集合操作 — 图片记忆库 CRUD 与查询
// =============================================================================

// MemoryAddImage 向指定 image 类型集合添加一条图片记录
// 接收 base64 图片数据与三个文本描述（情绪、色彩风格、主要内容），
// 分别调用嵌入模型对三个描述计算嵌入向量，然后按顺序存储为三元嵌入向量
// 返回新生成的 UUID 文档 ID
func (d *MemoryDB) MemoryAddImage(ctx context.Context, collectionName string, base64Image string, emotionDesc string, colorStyleDesc string, contentDesc string) (string, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	if c.CollectionType != CollectionTypeImage {
		return "", fmt.Errorf("集合 [%s] 类型为 %s, 不支持图片添加操作", collectionName, c.CollectionType)
	}

	if strings.TrimSpace(base64Image) == "" {
		return "", fmt.Errorf("图片数据不能为空")
	}

	// 批量嵌入三个文本描述，减少 API 调用次数
	texts := []string{emotionDesc, colorStyleDesc, contentDesc}
	vectors, err := d.embedTexts(ctx, c.Model, texts)
	if err != nil {
		return "", fmt.Errorf("嵌入图片描述文本失败: %v", err)
	}

	// 验证三个嵌入向量维度均与集合锁定维度一致
	for idx, vec := range vectors {
		if len(vec) != c.Dimension {
			return "", fmt.Errorf("嵌入向量[%d] 维度 %d 与集合 [%s] 维度 %d 不符",
				idx, len(vec), collectionName, c.Dimension)
		}
	}

	id := generateUUID()
	var embeddings [3][]float32
	copy(embeddings[:], vectors)

	doc := ImageDocument{
		ID:         id,
		Image:      base64Image,
		Embeddings: embeddings,
	}

	c.mu.Lock()
	c.ImageDocuments = append(c.ImageDocuments, doc)
	c.mu.Unlock()

	c.saveImageDocumentsToFile()

	logger.Info("Storage", "图片集合 [%s] 新增图片文档, ID: %s, 情绪: %s, 色彩: %s, 内容: %s",
		collectionName, id, truncateDesc(emotionDesc), truncateDesc(colorStyleDesc), truncateDesc(contentDesc))
	return id, nil
}

// MemoryQueryImages 按查询文本检索最相似的图片，采用三元嵌入向量 + topK 加权排序
// 查询流程：
//  1. 将查询文本嵌入为查询向量
//  2. 对每个条目，分别计算查询向量与三个嵌入向量的相似度，取平均为基础评分
//  3. 对每个向量索引（0/1/2），独立计算 topK 排名，按命中数加权基础评分
//     - 命中 0 个：×1.0 | 命中 1 个：×1.3 | 命中 2 个：×1.6 | 命中 3 个：×2.0
//  4. 按最终评分降序排列，返回前 topK 条结果
func (d *MemoryDB) MemoryQueryImages(ctx context.Context, collectionName string, queryText string, topK int) ([]ImageQueryResult, error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	if c.CollectionType != CollectionTypeImage {
		return nil, fmt.Errorf("集合 [%s] 类型为 %s, 不支持图片查询操作", collectionName, c.CollectionType)
	}

	if topK <= 0 {
		topK = 5
	}

	// 步骤 1：嵌入查询文本
	queryVec, err := d.embedText(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("嵌入查询文本失败: %v", err)
	}

	if len(queryVec) != c.Dimension {
		return nil, fmt.Errorf("查询嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(queryVec), collectionName, c.Dimension)
	}

	return c.queryImagesTopK(queryVec, topK), nil
}

// queryImagesTopK 按 queryVec 对图片文档执行三元嵌入向量 + topK 加权检索
// 返回按最终评分降序排列的前 topK 条结果
func (c *Collection) queryImagesTopK(queryVec []float32, topK int) []ImageQueryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	docs := c.ImageDocuments
	if len(docs) == 0 || topK <= 0 {
		return nil
	}

	const numVectors = 3

	// 步骤 2：计算每个条目与三个嵌入向量的相似度
	type entryScore struct {
		index  int
		scores [numVectors]float32 // 三个相似度：[情绪, 色彩风格, 主要内容]
	}
	entries := make([]entryScore, len(docs))
	for i := range docs {
		entries[i].index = i
		for v := 0; v < numVectors; v++ {
			entries[i].scores[v] = cosineSimilarity(queryVec, docs[i].Embeddings[v])
		}
	}

	// 步骤 3：对每个向量索引独立计算 topK 排名
	// 使用 entry index 作为唯一标识的集合来判断是否在 topK 中
	type topRankSet map[int]struct{}

	topRankSets := [numVectors]topRankSet{}
	for v := 0; v < numVectors; v++ {
		// 按该向量索引的相似度排序
		vecScored := make([]struct {
			index int
			score float32
		}, len(docs))
		for i := range docs {
			vecScored[i].index = i
			vecScored[i].score = entries[i].scores[v]
		}
		sort.SliceStable(vecScored, func(i, j int) bool {
			return vecScored[i].score > vecScored[j].score
		})

		// 取前 topK（或全部，如果文档数不足 topK）
		limit := topK
		if limit > len(vecScored) {
			limit = len(vecScored)
		}
		topRankSets[v] = make(topRankSet, limit)
		for k := 0; k < limit; k++ {
			topRankSets[v][vecScored[k].index] = struct{}{}
		}
	}

	// 步骤 4：计算最终评分
	// boostMultiplier: 0→1.0, 1→1.3, 2→1.6, 3→2.0
	boostMultiplier := [4]float32{1.0, 1.3, 1.6, 2.0}

	results := make([]ImageQueryResult, len(docs))
	for i := range entries {
		doc := &docs[entries[i].index]

		// 基础评分 = 三个相似度的平均值
		var baseScore float32
		for v := 0; v < numVectors; v++ {
			baseScore += entries[i].scores[v]
		}
		baseScore /= float32(numVectors)

		// 计算 topK 命中数
		hitCount := 0
		for v := 0; v < numVectors; v++ {
			if _, ok := topRankSets[v][entries[i].index]; ok {
				hitCount++
			}
		}

		finalScore := baseScore * boostMultiplier[hitCount]

		results[i] = ImageQueryResult{
			ID:         doc.ID,
			Image:      doc.Image,
			BaseScore:  baseScore,
			FinalScore: finalScore,
			BoostLevel: hitCount,
		}
	}

	// 按最终评分降序排序
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	if topK > len(results) {
		topK = len(results)
	}

	return results[:topK]
}

// truncateDesc 截断描述文本用于日志输出（最多 30 个字符）
func truncateDesc(s string) string {
	runes := []rune(s)
	if len(runes) <= 30 {
		return s
	}
	return string(runes[:30]) + "..."
}

// =============================================================================
// 全局包装函数 — 记忆库（多集合架构）
// =============================================================================

// IsInitialized 全局包装 — 返回记忆库实例是否已初始化
func IsInitialized() bool {
	return MemoryDatabase != nil && MemoryDatabase.IsMemoryInitialized()
}

// MemoryInitInstance 全局包装 — 初始化记忆库实例（不创建任何集合）
// 若 MemoryDatabase 为 nil，先调用 InitMemoryDB 准备存储目录
func MemoryInitInstance(baseURL string, apiKey string) error {
	if MemoryDatabase == nil {
		if err := InitMemoryDB(*config.MemoryDBDir); err != nil {
			return err
		}
	}
	return MemoryDatabase.MemoryInitInstance(baseURL, apiKey)
}

// CollectionInit 全局包装 — 创建或打开指定名称的 text 类型集合（探针定维度）
func CollectionInit(ctx context.Context, name string, modelName string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.CollectionInit(ctx, name, modelName)
}

// CollectionInitImage 全局包装 — 创建或打开指定名称的 image 类型集合（探针定维度）
func CollectionInitImage(ctx context.Context, name string, modelName string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.CollectionInitImage(ctx, name, modelName)
}

// AddImage 全局包装 — 向 image 集合添加图片记录，返回新生成的 UUID
func AddImage(ctx context.Context, collectionName string, base64Image string, emotionDesc string, colorStyleDesc string, contentDesc string) (string, error) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return "", fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryAddImage(ctx, collectionName, base64Image, emotionDesc, colorStyleDesc, contentDesc)
}

// QueryImages 全局包装 — 按查询文本检索图片，返回含加权评分的结构化结果
func QueryImages(ctx context.Context, collectionName string, queryText string, topK int) ([]ImageQueryResult, error) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return nil, fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryQueryImages(ctx, collectionName, queryText, topK)
}

// AddMessage 全局包装 — 添加消息（不返回 ID）
func AddMessage(ctx context.Context, collectionName string, role string, content string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryAddMessageSilent(ctx, collectionName, role, content)
}

// AddMessageWithID 全局包装 — 添加消息并返回新生成的 UUID 文档 ID
func AddMessageWithID(ctx context.Context, collectionName string, role string, content string) (string, error) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return "", fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryAddMessage(ctx, collectionName, role, content)
}

// QueryMessagesWithContent 全局包装 — 查询消息（返回含相似度的结构化结果）
func QueryMessagesWithContent(ctx context.Context, collectionName string, queryText string, topK int) ([]MemoryQueryResult, error) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return nil, fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryQueryMessagesWithContent(ctx, collectionName, queryText, topK)
}

// DeleteMessage 全局包装 — 按 ID 删除消息
func DeleteMessage(ctx context.Context, collectionName string, id string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryDeleteMessage(ctx, collectionName, id)
}

// GetCollectionCount 全局包装 — 返回集合文档总数
func GetCollectionCount(collectionName string) int {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return 0
	}
	return MemoryDatabase.MemoryGetCollectionCount(collectionName)
}

// GetDocuments 全局包装 — 分页返回文档条目
func GetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return []DocumentEntry{}, 0
	}
	return MemoryDatabase.MemoryGetDocuments(collectionName, offset, limit)
}

// GetEntryCount 全局包装 — 返回集合文档总数
func GetEntryCount(collectionName string) int {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return 0
	}
	return MemoryDatabase.MemoryGetEntryCount(collectionName)
}

// HasSyncMismatch 全局包装 — 检测维度不符文档
func HasSyncMismatch(collectionName string) bool {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return false
	}
	return MemoryDatabase.MemoryHasSyncMismatch(collectionName)
}

// DeleteCollection 全局包装 — 删除整个集合
func DeleteCollection(collectionName string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryDeleteCollection(collectionName)
}

// ClearCollection 全局包装 — 清空集合所有文档
func ClearCollection(collectionName string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryClearCollection(collectionName)
}

// RebuildEntries 全局包装 — 重建（删除维度不符文档）
func RebuildEntries(ctx context.Context, collectionName string) (int, error) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return 0, fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryRebuildEntries(ctx, collectionName)
}

// MemoryListCollections 全局包装 — 列出所有已加载集合名
func MemoryListCollections() []string {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return []string{}
	}
	return MemoryDatabase.MemoryListCollections()
}

// MemoryGetCollectionInfo 全局包装 — 获取集合的模型、维度、文档数
func MemoryGetCollectionInfo(collectionName string) (string, int, int, error) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return "", 0, 0, fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.MemoryGetCollectionInfo(collectionName)
}

// MemoryGetCollectionInfoWithType 全局包装 — 获取集合的模型、维度、文档数、类型
func MemoryGetCollectionInfoWithType(collectionName string) (string, int, int, string) {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return "", 0, 0, ""
	}
	return MemoryDatabase.MemoryGetCollectionInfoWithType(collectionName)
}

// cosineSimilarity 计算两个向量的余弦相似度，取值范围 [-1, 1]，越高越相似
// 当向量长度不一致或为零向量时返回 0
func cosineSimilarity(a, b []float32) float32 {
	n := len(a)
	if n != len(b) || n == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// queryTopK 按 queryVec 检索最相似的 topK 篇文档，按相似度降序返回
// 相似度相同的文档保持其在 Documents 中的原始顺序（稳定排序）
func (c *Collection) queryTopK(queryVec []float32, topK int) []MemoryQueryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Documents) == 0 || topK <= 0 {
		return nil
	}

	type scored struct {
		index int
		score float32
	}
	results := make([]scored, 0, len(c.Documents))
	for i := range c.Documents {
		results = append(results, scored{
			index: i,
			score: cosineSimilarity(queryVec, c.Documents[i].Embedding),
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}

	out := make([]MemoryQueryResult, topK)
	for i := 0; i < topK; i++ {
		doc := &c.Documents[results[i].index]
		out[i] = MemoryQueryResult{
			ID:         doc.ID,
			Role:       doc.Role,
			Content:    doc.Content,
			Similarity: results[i].score,
		}
	}
	return out
}
