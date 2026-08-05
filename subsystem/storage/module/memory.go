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

// CollectionInit 创建或打开指定名称的集合
// 通过探针文本嵌入一次确定向量维度，写入 metadata.json
// 若集合已存在且 model 一致则直接返回，model 变更则重新探针并更新维度
func (d *MemoryDB) CollectionInit(ctx context.Context, name string, modelName string) error {
	if !d.memoryInitialized {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	if err := validateCollectionName(name); err != nil {
		return err
	}

	// 已存在则直接返回
	d.collectionsMu.RLock()
	if _, ok := d.collections[name]; ok {
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
		Name:      name,
		Model:     meta.Model,
		Dimension: meta.Dimension,
		Documents: make([]MemoryDocument, 0),
		collDir:   collDir,
		metaPath:  metaPath,
	}
	c.loadDocumentsFromFile()

	d.collectionsMu.Lock()
	d.collections[name] = c
	d.collectionsMu.Unlock()

	logger.Info("Storage", "集合 [%s] 初始化完成, 模型: %s, 维度: %d, 文档数: %d",
		name, c.Model, c.Dimension, len(c.Documents))
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

		c := &Collection{
			Name:      name,
			Model:     meta.Model,
			Dimension: meta.Dimension,
			Documents: make([]MemoryDocument, 0),
			collDir:   collDir,
			metaPath:  metaPath,
		}
		c.loadDocumentsFromFile()

		d.collectionsMu.Lock()
		d.collections[name] = c
		d.collectionsMu.Unlock()

		logger.Info("Storage", "已加载集合 [%s], 模型: %s, 维度: %d, 文档数: %d",
			name, c.Model, c.Dimension, len(c.Documents))
	}
}

// IsMemoryInitialized 返回记忆库实例是否已初始化
func (d *MemoryDB) IsMemoryInitialized() bool {
	return d != nil && d.memoryInitialized
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
func (d *MemoryDB) MemoryDeleteMessage(ctx context.Context, collectionName string, id string) error {
	_ = ctx
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
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

// MemoryGetCollectionCount 返回集合中文档总数
func (d *MemoryDB) MemoryGetCollectionCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.reloadIfChanged()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Documents)
}

// MemoryGetDocuments 分页返回集合文档条目（不含嵌入向量），同时返回总数
func (d *MemoryDB) MemoryGetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return []DocumentEntry{}, 0
	}

	// 跨进程一致性检测：若磁盘文件被其他进程更新则重载内存缓存
	c.reloadIfChanged()

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
	return len(c.Documents)
}

// MemoryHasSyncMismatch 检测集合内是否有文档向量缺失或维度与集合锁定维度不符
func (d *MemoryDB) MemoryHasSyncMismatch(collectionName string) bool {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
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

	d.collectionsMu.Lock()
	delete(d.collections, collectionName)
	d.collectionsMu.Unlock()

	if err := os.RemoveAll(collDir); err != nil {
		return fmt.Errorf("删除集合目录失败: %v", err)
	}

	logger.Info("Storage", "集合 [%s] 已删除, 文档数: %d, 目录: %s",
		collectionName, len(c.Documents), collDir)
	return nil
}

// MemoryClearCollection 清空集合中所有文档（保留集合元数据，仅删除文档）
// 操作不可恢复，调用方应确保前端已做二次确认
func (d *MemoryDB) MemoryClearCollection(collectionName string) error {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return err
	}

	originalCount := len(c.Documents)

	c.mu.Lock()
	c.Documents = make([]MemoryDocument, 0)
	c.mu.Unlock()

	c.saveDocumentsToFile()

	logger.Info("Storage", "集合 [%s] 已清空, 删除文档数: %d",
		collectionName, originalCount)
	return nil
}

// MemoryRebuildEntries 删除向量缺失或维度不符的文档，重新持久化
// ctx 保留以兼容签名，当前实现不调用嵌入服务
func (d *MemoryDB) MemoryRebuildEntries(ctx context.Context, collectionName string) (int, error) {
	_ = ctx
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0, err
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
	return c.Model, c.Dimension, len(c.Documents), nil
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

// saveCollectionMeta 原子化写入集合元数据（含分块计数），并更新 lastFileModTime
// 在所有分块文件写入完成后调用，确保跨进程读取一致性
func (c *Collection) saveCollectionMeta() error {
	meta := collectionMeta{
		Model:      c.Model,
		Dimension:  c.Dimension,
		ChunkCount: c.chunkCount,
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
// 自动检测旧格式（documents.json）并即时迁移到新格式（分块存储）
// 兼容旧版 msg-N 格式 ID 与新版 UUID 格式 ID，加载时保留原值不做改写
func (c *Collection) loadDocumentsFromFile() {
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
// 按 MemoryChunkSize（100 条）切分文档，写入 contents_NNNN.json + embeddings_NNNN.json
// 自动清理多余分块文件（当文档总数减少时），最后原子化更新 metadata.json
func (c *Collection) saveDocumentsToFile() {
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

// CollectionInit 全局包装 — 创建或打开指定名称的集合（探针定维度）
func CollectionInit(ctx context.Context, name string, modelName string) error {
	if MemoryDatabase == nil || !MemoryDatabase.IsMemoryInitialized() {
		return fmt.Errorf("记忆库未初始化, 请先调用 MemoryInitInstance")
	}
	return MemoryDatabase.CollectionInit(ctx, name, modelName)
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
