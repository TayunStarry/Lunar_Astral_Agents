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
// 向量数据库初始化
// =============================================================================

// InitVectorDB 初始化向量数据库存储根目录
// 创建 baseDir、迁移旧 collections/ 层级、初始化集合 map，并赋值给全局 VectorDatabase 实例
// 此函数不产生网络请求，仅准备本地存储结构；嵌入服务连接由 VectorInitInstance 配置
func InitVectorDB(vectorDir string) error {
	if VectorDatabase != nil && VectorDatabase.vectorInitialized {
		return nil
	}

	if err := os.MkdirAll(vectorDir, 0755); err != nil {
		return fmt.Errorf("创建向量数据库目录失败: %v", err)
	}

	// 迁移旧版 collections/ 层级：将 <vectorDir>/collections/<name> 上移到 <vectorDir>/<name>
	migrateCollectionsLayer(vectorDir)

	db := &VectorDB{
		baseDir:     vectorDir,
		collections: make(map[string]*Collection),
	}
	VectorDatabase = db

	logger.Info("Storage", "向量数据库存储目录已就绪: %s", vectorDir)
	return nil
}

// migrateCollectionsLayer 迁移旧版 collections/ 层级到扁平化结构
// 若 <vectorDir>/collections/ 存在，将其下所有集合目录上移到 <vectorDir>/ 下
// 迁移完成后删除空的 collections/ 目录；已存在同名目录时跳过该集合
func migrateCollectionsLayer(vectorDir string) {
	oldCollectionsDir := filepath.Join(vectorDir, "collections")
	entries, err := os.ReadDir(oldCollectionsDir)
	if err != nil {
		// 旧目录不存在视为无需迁移
		return
	}

	migrated := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if validateCollectionName(name) != nil {
			continue
		}
		oldPath := filepath.Join(oldCollectionsDir, name)
		newPath := filepath.Join(vectorDir, name)

		// 目标已存在则跳过，避免覆盖用户数据
		if _, statErr := os.Stat(newPath); statErr == nil {
			logger.Warn("Storage", "迁移跳过: 目标集合目录已存在 %s", newPath)
			continue
		}

		if err := os.Rename(oldPath, newPath); err != nil {
			logger.Warn("Storage", "迁移集合 [%s] 失败: %v", name, err)
			continue
		}
		migrated++
		logger.Info("Storage", "迁移集合 [%s]: %s -> %s", name, oldPath, newPath)
	}

	// 尝试删除空的旧 collections 目录（非致命）
	if migrated > 0 {
		if rmErr := os.Remove(oldCollectionsDir); rmErr != nil {
			logger.Warn("Storage", "删除旧 collections 目录失败（可能非空）: %v", rmErr)
		} else {
			logger.Info("Storage", "已删除旧 collections/ 目录, 共迁移 %d 个集合", migrated)
		}
	}
}

// VectorInitInstance 初始化向量数据库实例（不创建任何集合）
// 仅配置嵌入服务连接，并加载已存在的集合到内存
// 方法接收者为 *VectorDB；全局包装函数同名 VectorInitInstance 负责实例懒初始化
func (d *VectorDB) VectorInitInstance(baseURL string, apiKey string) error {
	if d.vectorInitialized {
		return nil
	}

	if d.baseDir == "" {
		return fmt.Errorf("向量数据库未配置存储路径, 请先调用 InitVectorDB")
	}

	d.embeddingBaseURL = baseURL
	d.embeddingAPIKey = apiKey
	d.httpClient = &http.Client{Timeout: 120 * time.Second}
	d.vectorInitialized = true

	d.loadAllCollections()

	logger.Info("Storage", "向量数据库实例初始化完成, base_url: %s, 已加载 %d 个集合",
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
	// 拒绝 URL 路由保留名，避免与 /vector/init、/vector/stats、/vector/collections 冲突
	switch name {
	case "init", "stats", "collections":
		return fmt.Errorf("集合名不能使用保留字: %s", name)
	}
	return nil
}

// CollectionInit 创建或打开指定名称的集合
// 通过探针文本嵌入一次确定向量维度，写入 metadata.json
// 若集合已存在且 model 一致则直接返回，model 变更则重新探针并更新维度
func (d *VectorDB) CollectionInit(ctx context.Context, name string, modelName string) error {
	if !d.vectorInitialized {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
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

	filePath := filepath.Join(collDir, "documents.json")
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
		if err := saveCollectionMeta(metaPath, meta); err != nil {
			return fmt.Errorf("写入 metadata.json 失败: %v", err)
		}
	}

	c := &Collection{
		Name:      name,
		Model:     meta.Model,
		Dimension: meta.Dimension,
		Documents: make([]VectorDocument, 0),
		filePath:  filePath,
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

// saveCollectionMeta 写入集合元数据
func saveCollectionMeta(metaPath string, meta collectionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// getCollection 获取集合实例，不存在返回错误
func (d *VectorDB) getCollection(name string) (*Collection, error) {
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
func (d *VectorDB) loadAllCollections() {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "扫描向量存储目录失败: %v", err)
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
		filePath := filepath.Join(collDir, "documents.json")

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
			Documents: make([]VectorDocument, 0),
			filePath:  filePath,
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

// IsVectorInitialized 返回向量数据库实例是否已初始化
func (d *VectorDB) IsVectorInitialized() bool {
	return d != nil && d.vectorInitialized
}

// =============================================================================
// 向量数据库操作（多集合）
// =============================================================================

// VectorAddMessage 向指定集合添加一条消息，返回新生成的 UUID 文档 ID
// ID 采用 UUID v4 格式；旧版 msg-N 格式 ID 仅在历史数据加载时保留，新增文档一律使用 UUID
func (d *VectorDB) VectorAddMessage(ctx context.Context, collectionName string, role string, content string) (string, error) {
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
	c.Documents = append(c.Documents, VectorDocument{
		ID:        id,
		Role:      role,
		Content:   content,
		Embedding: embedding,
	})
	c.mu.Unlock()

	c.saveDocumentsToFile()
	return id, nil
}

// VectorAddMessageSilent 添加消息但不返回 ID，仅返回错误
func (d *VectorDB) VectorAddMessageSilent(ctx context.Context, collectionName string, role string, content string) error {
	_, err := d.VectorAddMessage(ctx, collectionName, role, content)
	return err
}

// VectorQueryMessages 按查询文本检索最相似的消息，返回向量消息兼容格式的 JSON 字符串列表
func (d *VectorDB) VectorQueryMessages(ctx context.Context, collectionName string, queryText string, topK int) ([]string, error) {
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
		msg := vectorMessage{Role: r.Role, Content: r.Content}
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		messages = append(messages, string(jsonBytes))
	}
	return messages, nil
}

// VectorQueryMessagesWithContent 按查询文本检索最相似的消息，返回含 ID/角色/内容/相似度的结构化结果
func (d *VectorDB) VectorQueryMessagesWithContent(ctx context.Context, collectionName string, queryText string, topK int) ([]VectorQueryResult, error) {
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

// VectorDeleteMessage 按 ID 删除指定集合中的一条文档
// 兼容 UUID 与旧版 msg-N 两种 ID 格式（按字符串相等匹配）
func (d *VectorDB) VectorDeleteMessage(ctx context.Context, collectionName string, id string) error {
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

// VectorGetCollectionCount 返回集合中文档总数
func (d *VectorDB) VectorGetCollectionCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Documents)
}

// VectorGetDocuments 分页返回集合文档条目（不含嵌入向量），同时返回总数
func (d *VectorDB) VectorGetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return []DocumentEntry{}, 0
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

// VectorGetEntryCount 返回集合中文档总数（与 VectorGetCollectionCount 等价，语义别名）
func (d *VectorDB) VectorGetEntryCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Documents)
}

// VectorHasSyncMismatch 检测集合内是否有文档向量缺失或维度与集合锁定维度不符
func (d *VectorDB) VectorHasSyncMismatch(collectionName string) bool {
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

// VectorRebuildEntries 删除向量缺失或维度不符的文档，重新持久化
// ctx 保留以兼容签名，当前实现不调用嵌入服务
func (d *VectorDB) VectorRebuildEntries(ctx context.Context, collectionName string) (int, error) {
	_ = ctx
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	original := len(c.Documents)
	filtered := make([]VectorDocument, 0, original)
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

// VectorListCollections 返回所有已加载集合的名称
func (d *VectorDB) VectorListCollections() []string {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	names := make([]string, 0, len(d.collections))
	for name := range d.collections {
		names = append(names, name)
	}
	return names
}

// VectorGetCollectionInfo 返回集合元信息（模型、维度、文档数）
func (d *VectorDB) VectorGetCollectionInfo(collectionName string) (model string, dimension int, count int, err error) {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return "", 0, 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Model, c.Dimension, len(c.Documents), nil
}

// =============================================================================
// Collection 持久化方法
// =============================================================================

// loadDocumentsFromFile 从 documents.json 加载文档到集合内存
// 兼容旧版 msg-N 格式 ID 与新版 UUID 格式 ID，加载时保留原值不做改写
func (c *Collection) loadDocumentsFromFile() {
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "集合 [%s] 读取 documents.json 失败: %v", c.Name, err)
		}
		return
	}

	if len(data) == 0 {
		return
	}

	var docs []VectorDocument
	if err := json.Unmarshal(data, &docs); err != nil {
		logger.Warn("Storage", "集合 [%s] documents.json 解析失败: %v", c.Name, err)
		return
	}

	c.mu.Lock()
	c.Documents = docs
	c.mu.Unlock()
}

// saveDocumentsToFile 原子化持久化文档：写临时文件 + rename
// Windows 上 rename 不能覆盖已存在文件，先 Remove 再 Rename
func (c *Collection) saveDocumentsToFile() {
	c.mu.RLock()
	data, err := json.MarshalIndent(c.Documents, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		logger.Error("Storage", "集合 [%s] documents 序列化失败: %v", c.Name, err)
		return
	}

	tmpPath := c.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		logger.Error("Storage", "集合 [%s] 临时文件写入失败: %v", c.Name, err)
		return
	}

	// Windows: Remove + Rename 模拟原子替换
	os.Remove(c.filePath)
	if err := os.Rename(tmpPath, c.filePath); err != nil {
		logger.Error("Storage", "集合 [%s] 原子重命名失败: %v", c.Name, err)
	}
}

// =============================================================================
// 全局包装函数 — 向量数据库（多集合架构）
// =============================================================================

// IsInitialized 全局包装 — 返回向量数据库实例是否已初始化
func IsInitialized() bool {
	return VectorDatabase != nil && VectorDatabase.IsVectorInitialized()
}

// VectorInitInstance 全局包装 — 初始化向量实例（不创建任何集合）
// 若 VectorDatabase 为 nil，先调用 InitVectorDB 准备存储目录
func VectorInitInstance(baseURL string, apiKey string) error {
	if VectorDatabase == nil {
		if err := InitVectorDB(*config.VectorDBDir); err != nil {
			return err
		}
	}
	return VectorDatabase.VectorInitInstance(baseURL, apiKey)
}

// CollectionInit 全局包装 — 创建或打开指定名称的集合（探针定维度）
func CollectionInit(ctx context.Context, name string, modelName string) error {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.CollectionInit(ctx, name, modelName)
}

// AddMessage 全局包装 — 添加消息（不返回 ID）
func AddMessage(ctx context.Context, collectionName string, role string, content string) error {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.VectorAddMessageSilent(ctx, collectionName, role, content)
}

// AddMessageWithID 全局包装 — 添加消息并返回新生成的 UUID 文档 ID
func AddMessageWithID(ctx context.Context, collectionName string, role string, content string) (string, error) {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return "", fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.VectorAddMessage(ctx, collectionName, role, content)
}

// QueryMessagesWithContent 全局包装 — 查询消息（返回含相似度的结构化结果）
func QueryMessagesWithContent(ctx context.Context, collectionName string, queryText string, topK int) ([]VectorQueryResult, error) {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return nil, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.VectorQueryMessagesWithContent(ctx, collectionName, queryText, topK)
}

// DeleteMessage 全局包装 — 按 ID 删除消息
func DeleteMessage(ctx context.Context, collectionName string, id string) error {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.VectorDeleteMessage(ctx, collectionName, id)
}

// GetCollectionCount 全局包装 — 返回集合文档总数
func GetCollectionCount(collectionName string) int {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return 0
	}
	return VectorDatabase.VectorGetCollectionCount(collectionName)
}

// GetDocuments 全局包装 — 分页返回文档条目
func GetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return []DocumentEntry{}, 0
	}
	return VectorDatabase.VectorGetDocuments(collectionName, offset, limit)
}

// GetEntryCount 全局包装 — 返回集合文档总数
func GetEntryCount(collectionName string) int {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return 0
	}
	return VectorDatabase.VectorGetEntryCount(collectionName)
}

// HasSyncMismatch 全局包装 — 检测维度不符文档
func HasSyncMismatch(collectionName string) bool {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return false
	}
	return VectorDatabase.VectorHasSyncMismatch(collectionName)
}

// RebuildEntries 全局包装 — 重建（删除维度不符文档）
func RebuildEntries(ctx context.Context, collectionName string) (int, error) {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return 0, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.VectorRebuildEntries(ctx, collectionName)
}

// VectorListCollections 全局包装 — 列出所有已加载集合名
func VectorListCollections() []string {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return []string{}
	}
	return VectorDatabase.VectorListCollections()
}

// VectorGetCollectionInfo 全局包装 — 获取集合的模型、维度、文档数
func VectorGetCollectionInfo(collectionName string) (string, int, int, error) {
	if VectorDatabase == nil || !VectorDatabase.IsVectorInitialized() {
		return "", 0, 0, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return VectorDatabase.VectorGetCollectionInfo(collectionName)
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
func (c *Collection) queryTopK(queryVec []float32, topK int) []VectorQueryResult {
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

	out := make([]VectorQueryResult, topK)
	for i := 0; i < topK; i++ {
		doc := &c.Documents[results[i].index]
		out[i] = VectorQueryResult{
			ID:         doc.ID,
			Role:       doc.Role,
			Content:    doc.Content,
			Similarity: results[i].score,
		}
	}
	return out
}
