package persistence

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// UUID v4 生成 — 对应前端 TypeScript UUID() 实现
// 遵循 RFC 4122 版本 4（随机 UUID），格式 xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
// =============================================================================

// generateUUID 生成一个符RFC 4122 v4 标准UUID 字符
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		LoggerGeneral.Warn("FileManager", "crypto/rand 失败, 退化为时间UUID: %v", err)
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
// 初始记忆库实例与集合管理
// =============================================================================

// initMemoryDBInstance 创建记忆库数据库实例（内部使用）
func initMemoryDBInstance(baseDir string) *MemoryDB {
	return &MemoryDB{
		baseDir:     baseDir,
		collections: make(map[string]*Collection),
	}
}

// currentMemoryEmbeddingModel 当前记忆库使用的嵌入模型名。
//
// 新架构不再有独立的记忆库模型配置：记忆库与智能体共用 agent.embedding_*，
// 因此这里统一读取 AgentEmbeddingModel。
func currentMemoryEmbeddingModel() string {
	if model := strings.TrimSpace(*GeneralConfig.AgentEmbeddingModel); model != "" {
		return model
	}
	return "system-embedding"
}

// MemoryInitInstance 初始化记忆库实例：一次性加载全部集合，并补齐默认分组与集合名向量。
//
// 启动时不再指定某个集合：磁盘上的所有集合全部载入，默认分组集合缺失时自动创建，
// 没有集合名嵌入向量的集合按集合名调用嵌入模型补齐。
// 模型配置与智能体共用 agent.embedding_* / agent.multimodal_*。
func (d *MemoryDB) MemoryInitInstance() error {
	if d.baseDir == "" {
		return fmt.Errorf("记忆库 baseDir 为空，请先调用 InitMemoryDB")
	}
	if err := os.MkdirAll(d.baseDir, 0755); err != nil {
		return fmt.Errorf("创建记忆库目录失败: %w", err)
	}

	d.initMu.Lock()
	defer d.initMu.Unlock()

	if !d.memoryInitialized {
		d.httpClient = &http.Client{Timeout: 120 * time.Second}
		d.memoryInitialized = true
		d.loadAllCollections()
	}

	// 每次实例初始化都做一遍幂等补齐：默认分组、缺失的集合名向量
	// 嵌入服务启动较晚时，后续memoryInit 调用会在这里继续补齐，而不是永久停摆
	defaultsOK := d.ensureDefaultCollections()
	namesOK := d.ensureNameEmbeddings()

	if d.MemoryGetCollectionCount() == 0 {
		return fmt.Errorf("记忆库没有可用集合（默认分组创建失败，请确认嵌入服务可用后重试）")
	}
	if !defaultsOK || !namesOK {
		return fmt.Errorf("记忆库分组尚未全部就绪（嵌入服务可能未启动，稍后重试）")
	}
	return nil
}

// ensureDefaultCollections 确保默认分组集合存在（已存在则跳过），返回是否全部就
func (d *MemoryDB) ensureDefaultCollections() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ok := true
	for _, name := range DefaultMemoryCollections {
		d.collectionsMu.RLock()
		_, exists := d.collections[name]
		d.collectionsMu.RUnlock()
		if exists {
			continue
		}
		if err := d.CollectionInit(ctx, name); err != nil {
			LoggerGeneral.Warn("FileManager", "默认记忆分组 [%s] 初始化失败: %v（下次启动重试）", name, err)
			ok = false
			continue
		}
		LoggerGeneral.Info("FileManager", "默认记忆分组 [%s] 已就绪", name)
	}
	return ok
}

// ensureNameEmbeddings 为所有缺少集合名嵌入向量的集合补齐向量，返回是否全部就绪
func (d *MemoryDB) ensureNameEmbeddings() bool {
	d.collectionsMu.RLock()
	collections := make([]*Collection, 0, len(d.collections))
	for _, c := range d.collections {
		collections = append(collections, c)
	}
	d.collectionsMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ok := true
	for _, c := range collections {
		c.mu.RLock()
		ready := len(c.NameEmbedding) > 0 && (c.NameEmbeddingModel == "" || c.NameEmbeddingModel == c.Model)
		c.mu.RUnlock()
		if ready {
			continue
		}
		if err := d.ensureCollectionNameEmbedding(ctx, c); err != nil {
			LoggerGeneral.Warn("FileManager", "集合 [%s] 名称嵌入生成失败: %v（下次启动重试）", c.Name, err)
			ok = false
			continue
		}
		LoggerGeneral.Info("FileManager", "集合 [%s] 名称嵌入向量已生成（模型 %s）", c.Name, c.NameEmbeddingModel)
	}
	return ok
}

// ensureCollectionNameEmbedding 为单个集合补齐名称嵌入向量
func (d *MemoryDB) ensureCollectionNameEmbedding(ctx context.Context, c *Collection) error {
	c.mu.RLock()
	ready := len(c.NameEmbedding) > 0 && (c.NameEmbeddingModel == "" || c.NameEmbeddingModel == c.Model)
	model := c.Model
	name := c.Name
	c.mu.RUnlock()
	if ready {
		return nil
	}
	if model == "" {
		model = currentMemoryEmbeddingModel()
	}
	vec, err := d.embedText(ctx, model, name)
	if err != nil {
		return fmt.Errorf("集合名嵌入失败: %w", err)
	}
	c.mu.Lock()
	c.NameEmbedding = vec
	c.NameEmbeddingModel = model
	if c.Dimension == 0 {
		c.Dimension = len(vec)
	}
	c.mu.Unlock()
	return c.saveCollectionMeta()
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

// CollectionInit 初始化或打开一个分组集合。
//
// 集合是内容自动归入的语义分组：创建时立即用集合名调用嵌入模型，生成集合名向量。
func (d *MemoryDB) CollectionInit(ctx context.Context, name string) error {
	if err := d.isCollectionValid(name); err != nil {
		return err
	}

	// 已在缓存：补齐磁盘目录与集合名向量即
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
	modelName := currentMemoryEmbeddingModel()

	// 已存在元数据：按现有数据加载（模型保持集合自身锁定的值，不再因配置变更清空用户记忆）
	recreate := false
	if _, err := os.Stat(metaPath); err == nil {
		var meta collectionMeta
		if err := readJSONFile(metaPath, &meta); err != nil {
			return fmt.Errorf("读取元数据失败: %w", err)
		}
		if meta.Version < CurrentVersion || meta.EmbeddingModel == "" || meta.EmbeddingDimension == 0 {
			LoggerGeneral.Warn("FileManager", "集合 %s 元数据不满足新格式 (version=%d, model=%s, dim=%d)，清空重建",
				name, meta.Version, meta.EmbeddingModel, meta.EmbeddingDimension)
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
				Model:              meta.EmbeddingModel,
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
		Model:              modelName,
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
func (d *MemoryDB) getCollection(name string) (*Collection, error) {
	d.collectionsMu.RLock()
	c, ok := d.collections[name]
	d.collectionsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("集合 '%s' 不存在，请先初始化", name)
	}
	return c, nil
}

// snapshotCollections 返回全部集合的快照（用于全局检索与内容归组）
func (d *MemoryDB) snapshotCollections() []*Collection {
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

		// v5 版本检
		if meta.Version < CurrentVersion {
			LoggerGeneral.Warn("FileManager", "集合 %s 版本过低 (%d < %d)，跳过加载", name, meta.Version, CurrentVersion)
			continue
		}
		if meta.EmbeddingModel == "" || meta.EmbeddingDimension == 0 {
			LoggerGeneral.Warn("FileManager", "集合 %s 元数据不完整，跳过加载", name)
			continue
		}

		c := &Collection{
			Name:               name,
			Model:              meta.EmbeddingModel,
			Dimension:          meta.EmbeddingDimension,
			NameEmbedding:      meta.NameEmbedding,
			NameEmbeddingModel: meta.NameEmbeddingModel,
			collDir:            filepath.Join(d.baseDir, name),
			metaPath:           metaPath,
		}

		if err := c.loadDocumentsFromFile(); err != nil {
			LoggerGeneral.Error("FileManager", "加载集合 %s 文档失败: %v", name, err)
			continue
		}
		c.updateLastModTime()

		d.collectionsMu.Lock()
		d.collections[name] = c
		d.collectionsMu.Unlock()

		LoggerGeneral.Info("FileManager", "已加载集合 %s (%d 文档, 名称向量=%v)", name, len(c.Documents), len(c.NameEmbedding) > 0)
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

// findDuplicateByEmbedding 基于内容嵌入向量判定是否与既有记忆重复。
// 余弦相似度 ≥ MemoryDedupThreshold 视为重复，返回已存在文档 ID；否则返回空字符串。
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

// ErrMemoryEntryNotFound 指定 ID 的记忆条目不存在
var ErrMemoryEntryNotFound = errors.New("记忆条目不存在")

// memoryDocumentType 返回文档的记忆类型；旧数据没有 type 字段时按内容推断
func memoryDocumentType(doc Document) string {
	if doc.Type != "" {
		return doc.Type
	}
	if doc.URL != "" {
		lower := strings.ToLower(doc.URL)
		if strings.Contains(lower, "/audios/") || strings.Contains(lower, ".mp3") ||
			strings.Contains(lower, ".wav") || strings.Contains(lower, ".ogg") ||
			strings.Contains(lower, ".m4a") || strings.Contains(lower, ".flac") {
			return MemoryTypeAudio
		}
		return MemoryTypeImage
	}
	if doc.Base64 != "" {
		return MemoryTypeImage
	}
	return MemoryTypeText
}

// NormalizeMemoryType 校验并归一化记忆类型；空值默认按文本记忆处理
func NormalizeMemoryType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", MemoryTypeText:
		return MemoryTypeText, nil
	case MemoryTypeImage:
		return MemoryTypeImage, nil
	case MemoryTypeAudio:
		return MemoryTypeAudio, nil
	default:
		return "", fmt.Errorf("未知的记忆类型: %q（支持 text/image/audio）", value)
	}
}

// memoryResultOf 把文档转换为查询/列举结果（附带所属集合与类型
func memoryResultOf(c *Collection, doc *Document, similarity float32) MemoryQueryResult {
	docType := memoryDocumentType(*doc)
	role := doc.Role
	if role == "" && docType != MemoryTypeText {
		role = docType
	}
	return MemoryQueryResult{
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
func (d *MemoryDB) routeCollection(ctx context.Context, content string) (*Collection, []float32, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil, fmt.Errorf("用于归组的内容为空")
	}

	collections := d.snapshotCollections()
	vectors := make(map[string][]float32)
	var best *Collection
	var bestVector []float32
	bestSim := float32(-2)

	for _, c := range collections {
		c.mu.RLock()
		name := c.Name
		model := c.Model
		nameEmbedding := append([]float32(nil), c.NameEmbedding...)
		c.mu.RUnlock()
		if model == "" || len(nameEmbedding) == 0 {
			continue
		}

		vec, ok := vectors[model]
		if !ok {
			embedded, err := d.embedText(ctx, model, content)
			if err != nil {
				LoggerGeneral.Warn("FileManager", "集合 [%s] 内容归组嵌入失败（模型 %s）: %v", name, model, err)
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
			c.mu.RLock()
			model := c.Model
			c.mu.RUnlock()
			if model == "" {
				continue
			}
			vec, err := d.embedText(ctx, model, content)
			if err != nil {
				continue
			}
			LoggerGeneral.Warn("FileManager", "集合名向量缺失，内容暂存到分组 [%s]（待下次启动补齐后恢复归组）", c.Name)
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

// MemoryAddMessage 添加文本消息到记忆库（内容自动归入最相关的分组集合）
func (d *MemoryDB) MemoryAddMessage(ctx context.Context, role, content string) (string, error) {
	return d.MemoryAddMessageWithTags(ctx, role, content, nil, "")
}

// MemoryAddMessageWithTags 添加文本消息并携带预设标签（写入来源可标记，如 action）。
//
// 标签文本只用于嵌入与归组；实际对话正文仍存入 content 字段。
func (d *MemoryDB) MemoryAddMessageWithTags(ctx context.Context, role, content string, tags []string, source string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("消息内容不能为空")
	}

	// 1. 确定用于嵌入与归组的文本：有预设标签用标签拼接文本，否则用正文全
	embeddingText := content
	if len(tags) > 0 {
		embeddingText = formatTagsText(tags)
	}

	// 2. 计算内容向量并自动归
	c, contentVec, err := d.routeCollection(ctx, embeddingText)
	if err != nil {
		return "", err
	}

	// 3. 仅使用内容嵌入向量判定是否重
	if existingID := c.findDuplicateByEmbedding(contentVec); existingID != "" {
		LoggerGeneral.Info("FileManager", "集合 %s 已存在内容相近的文本记忆 (ID=%s)，跳过重复写入", c.Name, existingID)
		return existingID, nil
	}

	// 4. 写入并持久化（content 保存实际文本内容，标签文本仅用于嵌入
	id := generateUUID()
	if err := c.appendDocument(Document{
		ID:        id,
		Type:      MemoryTypeText,
		Role:      role,
		Content:   content,
		Source:    source,
		Embedding: contentVec,
		Timestamp: time.Now().Unix(),
	}); err != nil {
		return "", err
	}
	return id, nil
}

// MemoryAddImage 添加图片到记忆库（自动归组）。
//
// 调用多模态模型生成画面标签，以标签文本计算内容向量并选择分组；
// 图片实体落到 multimedia/memory/<集合名>/，记忆库只保存 URL。
func (d *MemoryDB) MemoryAddImage(ctx context.Context, base64Image string, orientation string, custom string) (string, error) {
	if strings.TrimSpace(base64Image) == "" {
		return "", fmt.Errorf("图片数据不能为空")
	}

	// 1. 感知者式标准化：得到入库 URI（静态图缩放/转码；动态图保留动画）与视频理解引用
	storeURI, mediaRefs, animated := standardizeMemoryImage(base64Image)

	// 2. 多模态标签化理解（单线程 + 同步阻塞）：动态图file:// 视频链路，静态图走单
	d.llmMu.Lock()
	var tags []string
	var err error
	if animated && len(mediaRefs) > 0 {
		tags, err = d.generateTagsFromMedia(ctx, mediaRefs, orientation, custom)
		if err != nil {
			// 视频链路失败（如非 llama-server 后端）：回退单图理解再试一轮。
			// 优先用首帧而不是整张动图：多数多模态模型只可靠解码静态帧。
			LoggerGeneral.Warn("Persistence", "动态图帧序列理解失败（%v），回退单图理解", err)
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

	// 3. 以标签文本计算内容向量并自动归组
	tagsText := formatTagsText(tags)
	if tagsText == "" {
		tagsText = "图片"
	}
	c, contentVec, err := d.routeCollection(ctx, tagsText)
	if err != nil {
		return "", err
	}

	// 4. 图片实体落到 multimedia/memory/<集合名>/，记忆库只保存 URL（不再内联 base64）
	url, err := saveMemoryImageFile(c.Name, storeURI)
	if err != nil {
		return "", fmt.Errorf("图片落盘失败: %w", err)
	}

	// 5. 同一张图片（内容哈希相同）只保留一条记忆；不同图片即使标签相同也分别入库
	if existingID := c.findDuplicateByURL(url); existingID != "" {
		LoggerGeneral.Info("FileManager", "集合 %s 已存在相同图片记忆 (ID=%s)，跳过重复写入", c.Name, existingID)
		return existingID, nil
	}

	id := generateUUID()
	if err := c.appendDocument(Document{
		ID:        id,
		Type:      MemoryTypeImage,
		URL:       url,
		Embedding: contentVec,
		Timestamp: time.Now().Unix(),
	}); err != nil {
		return "", err
	}
	return id, nil
}

// MemoryAddAudio 添加音频记忆（自动归组）。
//
// 音频本身不做内容理解，使用调用方给出的描述文本计算嵌入与归组；描述为空时按“音频”处理。
func (d *MemoryDB) MemoryAddAudio(ctx context.Context, base64Audio string, description string) (string, error) {
	if strings.TrimSpace(base64Audio) == "" {
		return "", fmt.Errorf("音频数据不能为空")
	}
	embeddingText := strings.TrimSpace(description)
	if embeddingText == "" {
		embeddingText = "音频"
	}
	c, contentVec, err := d.routeCollection(ctx, embeddingText)
	if err != nil {
		return "", err
	}
	url, err := saveMemoryAudioFile(c.Name, base64Audio)
	if err != nil {
		return "", fmt.Errorf("音频落盘失败: %w", err)
	}
	// 同一份音频（内容哈希相同）只保留一条记忆
	if existingID := c.findDuplicateByURL(url); existingID != "" {
		return existingID, nil
	}
	id := generateUUID()
	if err := c.appendDocument(Document{
		ID:        id,
		Type:      MemoryTypeAudio,
		Content:   strings.TrimSpace(description),
		URL:       url,
		Embedding: contentVec,
		Timestamp: time.Now().Unix(),
	}); err != nil {
		return "", err
	}
	return id, nil
}

// saveMemoryAudioFile 把音频实体写入 audios/<集合名>/，返回可读取的 URL
func saveMemoryAudioFile(collectionName, dataURI string) (string, error) {
	// 必须在创建临时文件之前初始化资源目录：否则初始化时的存储扫描会把
	// SaveFileEx 刚创建的临时文件登记成正式实体，导致返回临时文件名。
	if err := EnsureArchiveInitialized(); err != nil {
		return "", err
	}
	mimeType, raw, err := stripDataURI(dataURI)
	if err != nil {
		return "", err
	}
	ext := audioFileExt(mimeType)
	sum := sha256.Sum256(raw)
	rel := path.Join(StorageRootAudios, collectionName, hex.EncodeToString(sum[:])[:16]+ext)
	result, saveErr := SaveFileEx(rel, SaveOptions{Overwrite: true, MustStore: true}, bytes.NewReader(raw))
	if saveErr != nil {
		return "", saveErr
	}
	return "/file/read/" + result.RelPath, nil
}

// audioFileExt 音频 MIME → 扩展名
func audioFileExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/aac":
		return ".aac"
	case "audio/opus":
		return ".opus"
	case "audio/webm":
		return ".webm"
	default:
		return ".audio"
	}
}

// MemoryQuery 在所有集合中检索指定类型的记忆。
//
// 调用方只指定返回类型（text/image/audio），不再指定集合名；集合名只是内部分组。
func (d *MemoryDB) MemoryQuery(ctx context.Context, queryText, mediaType string, topK int) ([]MemoryQueryResult, error) {
	if topK <= 0 {
		return nil, nil
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return nil, nil
	}
	mediaType, err := NormalizeMemoryType(mediaType)
	if err != nil {
		return nil, err
	}

	collections := d.snapshotCollections()
	if len(collections) == 0 {
		return nil, nil
	}

	queryVectors := make(map[string][]float32)
	var results []MemoryQueryResult
	var lastErr error
	for _, c := range collections {
		c.reloadIfChanged()
		c.mu.RLock()
		model := c.Model
		c.mu.RUnlock()
		if model == "" {
			continue
		}
		vec, ok := queryVectors[model]
		if !ok {
			embedded, embedErr := d.embedQuery(ctx, model, queryText)
			if embedErr != nil {
				lastErr = embedErr
				LoggerGeneral.Warn("FileManager", "集合 [%s] 查询嵌入失败（模型 %s）: %v", c.Name, model, embedErr)
				continue
			}
			queryVectors[model] = embedded
			vec = embedded
		}
		results = append(results, c.queryTopK(vec, topK, mediaType)...)
	}

	if len(results) == 0 && lastErr != nil {
		return nil, fmt.Errorf("查询嵌入失败: %w", lastErr)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Similarity != results[j].Similarity {
			return results[i].Similarity > results[j].Similarity
		}
		return results[i].Timestamp > results[j].Timestamp
	})
	if topK > len(results) {
		topK = len(results)
	}
	return results[:topK], nil
}

// MemoryKeywordSearch 在全部集合中按关键词或正则扫描文本记忆（不调用嵌入服务）
//
// 与向量检索互补：
//   - 关键词模式：terms 全部命中（大小写不敏感的子串匹配），适合找人名、原话、专有名词；
//   - 正则模式：terms 以空格拼接后作为正则表达式（自动(?i) 忽略大小写）
//     适合精确模式匹配。Go regexp RE2 语法，不支持反向引用与环视
//
// 返回按入库时间倒序的结果；Similarity 固定0，由调用方用检索方式区分展示
func (d *MemoryDB) MemoryKeywordSearch(mediaType string, terms []string, useRegex bool, limit int) ([]MemoryQueryResult, error) {
	if limit <= 0 || len(terms) == 0 {
		return nil, nil
	}
	mediaType, err := NormalizeMemoryType(mediaType)
	if err != nil {
		return nil, err
	}

	var match func(string) bool
	if useRegex {
		re, err := regexp.Compile("(?i)" + strings.Join(terms, " "))
		if err != nil {
			return nil, fmt.Errorf("正则表达式无效: %w", err)
		}
		match = re.MatchString
	} else {
		lowered := make([]string, 0, len(terms))
		for _, term := range terms {
			if text := strings.ToLower(strings.TrimSpace(term)); text != "" {
				lowered = append(lowered, text)
			}
		}
		if len(lowered) == 0 {
			return nil, nil
		}
		match = func(text string) bool {
			lower := strings.ToLower(text)
			for _, term := range lowered {
				if !strings.Contains(lower, term) {
					return false
				}
			}
			return true
		}
	}

	var results []MemoryQueryResult
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		results = append(results, c.keywordResults(match, mediaType)...)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})
	if limit > len(results) {
		limit = len(results)
	}
	return results[:limit], nil
}

// keywordResults 在当前集合内按匹配函数扫描文本记忆
func (c *Collection) keywordResults(match func(string) bool, mediaType string) []MemoryQueryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make([]MemoryQueryResult, 0)
	for i := len(c.Documents) - 1; i >= 0; i-- {
		doc := c.Documents[i]
		if memoryDocumentType(doc) != mediaType {
			continue
		}
		// 媒体记忆没有正文，不参与关键词检
		if doc.Content == "" || !match(doc.Content) {
			continue
		}
		results = append(results, memoryResultOf(c, &doc, 0))
	}
	return results
}

// queryTopK 内容向量检索核心算
// 嵌入查询 与全量文档内容向量算余弦相似按类型过过滤低于 DocRankThreshold
// 按相似度降序（同分按插入顺序）返topK
func (c *Collection) queryTopK(queryVec []float32, topK int, mediaType string) []MemoryQueryResult {
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
	for i := range c.Documents {
		doc := &c.Documents[i]
		if len(doc.Embedding) == 0 || memoryDocumentType(*doc) != mediaType {
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
		results[i] = memoryResultOf(c, &c.Documents[rankedList[i].idx], rankedList[i].sim)
	}
	return results
}

// MemoryGetEntry 按 ID 返回一条记忆（跨全部集合查找）
func (d *MemoryDB) MemoryGetEntry(id string) (*MemoryQueryResult, error) {
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
			result := memoryResultOf(c, &c.Documents[i], 0)
			c.mu.RUnlock()
			return &result, nil
		}
		c.mu.RUnlock()
	}
	return nil, ErrMemoryEntryNotFound
}

// MemoryListEntries 逐一输出记忆条目（跨全部集合）。
//
// mediaType 为空表示不限类型；offset/limit 用于分页，limit <= 0 表示返回全部。
// 返回按入库时间倒序的条目与过滤后的总数。
func (d *MemoryDB) MemoryListEntries(mediaType string, offset, limit int) ([]MemoryQueryResult, int) {
	filter := strings.ToLower(strings.TrimSpace(mediaType))
	if filter != "" {
		if normalized, err := NormalizeMemoryType(filter); err == nil {
			filter = normalized
		} else {
			return nil, 0
		}
	}
	if offset < 0 {
		offset = 0
	}

	var entries []MemoryQueryResult
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		c.mu.RLock()
		for i := range c.Documents {
			doc := &c.Documents[i]
			if filter != "" && memoryDocumentType(*doc) != filter {
				continue
			}
			entries = append(entries, memoryResultOf(c, doc, 0))
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

// MemoryCountEntries 返回指定类型（空=全部）的记忆条目总数
func (d *MemoryDB) MemoryCountEntries(mediaType string) int {
	_, total := d.MemoryListEntries(mediaType, 0, 1)
	return total
}

// MemoryDeleteMessage 删除指定 UUID 的文档（跨全部集合查找）
func (d *MemoryDB) MemoryDeleteMessage(ctx context.Context, id string) error {
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
		// 删除条目时顺带清理其专属媒体实体（无其它条目引用时）
		d.removeMemoryMediaForDoc(c.Name, deleted)
		return nil
	}
	return ErrMemoryEntryNotFound
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
		docType := memoryDocumentType(doc)
		role := doc.Role
		if role == "" && docType != MemoryTypeText {
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

// MemoryGetEntryCount 返回指定集合的文档总数
func (d *MemoryDB) MemoryGetEntryCount(collectionName string) int {
	c, err := d.getCollection(collectionName)
	if err != nil {
		return 0
	}
	return c.docCount()
}

// MemoryReloadCollection 从磁盘重新加载指定分组。
// 供前端管理工具直接改写 documents_NNNN.json 后刷新进程内缓存（不重建向量）。
func (d *MemoryDB) MemoryReloadCollection(collectionName string) error {
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
			return true // 存在缺失内容向量的文
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

// MemoryClearCollection 清空集合所有数据，保留 metadata.json
func (d *MemoryDB) MemoryClearCollection(collectionName string) error {
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

// MemoryRebuildEntries 重建集合的文档内容嵌入向量：
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
	c.mu.RLock()
	effectiveModel := c.Model
	c.mu.RUnlock()
	if modelName != "" {
		effectiveModel = modelName
		c.mu.Lock()
		c.Model = modelName
		c.mu.Unlock()
	}

	for i, doc := range docs {
		if progress != nil {
			progress(i+1, len(docs))
		}

		var contentVec []float32
		if memoryDocumentType(doc) == MemoryTypeImage {
			// 图片记忆：重新多模态生成标签并以标签文本嵌入（URL 记忆先读回文件）
			imageSource := doc.Base64
			if imageSource == "" {
				imageSource = memoryImageDataURI(doc.URL)
			}
			d.llmMu.Lock()
			tags, terr := d.generateTags(ctx, imageSource, true, RecognitionAuto, "")
			d.llmMu.Unlock()
			if terr != nil {
				LoggerGeneral.Warn("FileManager", "重建图片标签失败 [%s]: %v", doc.ID, terr)
				continue
			}
			contentVec, err = d.embedText(ctx, effectiveModel, formatTagsText(tags))
		} else {
			// 文本 / 音频记忆：以正文（音频为描述文本）重新嵌
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
			LoggerGeneral.Warn("FileManager", "重建集合 [%s] 名称嵌入失败: %v", c.Name, err)
		}
	}
	if err := c.saveCollectionMeta(); err != nil {
		return fmt.Errorf("保存元数据失败: %w", err)
	}

	return nil
}

// memoryImageDataURI 把记忆图URL 读回data URI（重建标签向量使用）
func memoryImageDataURI(url string) string {
	rel := strings.TrimPrefix(strings.SplitN(url, "?", 2)[0], "/file/read/")
	if rel == "" || !strings.HasPrefix(rel, "multimedia/") {
		return url
	}
	abs := filepath.Join(filepath.Clean(*GeneralConfig.LocalDir), filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		return url
	}
	mimeType := "image/png"
	if entry, ok := previewAllowlist[strings.ToLower(filepath.Ext(abs))]; ok {
		mimeType = entry.MIME
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// MemoryListCollections 返回所有集合名
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
		"name":                     c.Name,
		"embedding_model":          c.Model,
		"embedding_dimension":      c.Dimension,
		"name_embedding_model":     c.NameEmbeddingModel,
		"name_embedding_dimension": len(c.NameEmbedding),
		"has_name_embedding":       len(c.NameEmbedding) > 0,
		"version":                  CurrentVersion,
		"document_count":           len(c.Documents),
		"documents_chunk_count":    c.documentsChunkCount,
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

// readJSONFile 读取 JSON 文件并反序列化到目标结构
func readJSONFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// atomicWriteJSON 原子写入 JSON 文件（先写临时文件，再重命名；单行紧凑存储，不格式化
func atomicWriteJSON(path string, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	// 确保目标目录存在：集合目录可能被外部清理或重建遗漏，缺失时自动创建，避免写临时文件失
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
	// 直接 rename 会偶"Access is denied"。先短暂重试，再尝试移除目标后重命名
	if err := substituteRename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// substituteRename 替换式重命名：短暂重+ 移除目标兜底，规Windows 目标文件占用导致 Access denied
func substituteRename(tmpPath, path string) error {
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.Rename(tmpPath, path); err == nil {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	// 兜底：先移除旧目标再重命
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
	c.mu.RLock()
	meta := collectionMeta{
		EmbeddingModel:       c.Model,
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

// loadDocumentsFromFile 从分块文件加载文档与嵌入向量到内
// documents_NNNN.json 存文档本体（不含向量），embeddings_NNNN.json 存向量并ID 关联
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
			allDocs[j].Type = memoryDocumentType(allDocs[j])
		}
	}

	c.mu.Lock()
	c.Documents = allDocs
	c.mu.Unlock()

	// 历史数据迁移：
	//  1. 旧版逐条内联图片 base64 → 落盘为内容仓库实体，JSON 只保留 URL；
	//  2. 旧版 multimedia/<集合名>/ → multimedia/memory/<集合名>/。
	// 失败的下次加载再试，不阻断集合可用。
	changed := c.migrateInlineImages()
	if c.migrateLegacyMemoryImages() {
		changed = true
	}
	if changed {
		_ = c.saveDocumentsToFile()
	}

	return nil
}

// migrateInlineImages 把旧文档里的 base64 图片迁移为内容仓库文件，返回是否有变
func (c *Collection) migrateInlineImages() bool {
	c.mu.RLock()
	docs := append([]Document(nil), c.Documents...)
	c.mu.RUnlock()

	changed := false
	for i := range docs {
		if docs[i].Base64 == "" || docs[i].URL != "" {
			continue
		}
		url, err := saveMemoryImageFile(c.Name, docs[i].Base64)
		if err != nil {
			LoggerGeneral.Warn("Persistence", "记忆图片迁移失败 [%s]: %v", docs[i].ID, err)
			continue
		}
		c.mu.Lock()
		for j := range c.Documents {
			if c.Documents[j].ID == docs[i].ID {
				c.Documents[j].URL = url
				c.Documents[j].Base64 = ""
				break
			}
		}
		c.mu.Unlock()
		changed = true
	}
	return changed
}

// saveDocumentsToFile 将文档与嵌入向量保存到分块文件（100 块）
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

	// 写入各分块（文档 + 嵌入向量
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

	// 清理多余的分块文
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

// reloadIfChanged 检metadata.json 是否被外部进程修改，若是则重新加
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

// MemoryInitInstance 全局初始化记忆库实例（模型配置从 agent 分组读取
func MemoryInitInstance() error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryInitInstance()
}

// CollectionInit 全局初始化集合（创建时按集合名生成嵌入向量）
func CollectionInit(ctx context.Context, name string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.CollectionInit(ctx, name)
}

// MemoryAddMessage 全局添加文本消息（内容自动归组）
func MemoryAddMessage(ctx context.Context, role, content string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessage(ctx, role, content)
}

// MemoryAddMessageWithTags 全局添加文本消息（携带预设标签与来源标记）
func MemoryAddMessageWithTags(ctx context.Context, role, content string, tags []string, source string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddMessageWithTags(ctx, role, content, tags, source)
}

// MemoryAddImage 全局添加图片（内容自动归组）
func MemoryAddImage(ctx context.Context, base64Image string, orientation string, custom string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddImage(ctx, base64Image, orientation, custom)
}

// MemoryAddAudio 全局添加音频（按描述文本归组）
func MemoryAddAudio(ctx context.Context, base64Audio string, description string) (string, error) {
	if globalMemoryDB == nil {
		return "", fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryAddAudio(ctx, base64Audio, description)
}

// MemoryQuery 全局按类型查询（跨全部集合）
func MemoryQuery(ctx context.Context, queryText, mediaType string, topK int) ([]MemoryQueryResult, error) {
	if globalMemoryDB == nil {
		return nil, fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryQuery(ctx, queryText, mediaType, topK)
}

// MemoryKeywordSearch 全局关键词/正则检索（不调用嵌入服务）
func MemoryKeywordSearch(mediaType string, terms []string, useRegex bool, limit int) ([]MemoryQueryResult, error) {
	if globalMemoryDB == nil {
		return nil, fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryKeywordSearch(mediaType, terms, useRegex, limit)
}

// MemoryGetEntry 全局按 ID 获取条目（跨全部集合）
func MemoryGetEntry(id string) (*MemoryQueryResult, error) {
	if globalMemoryDB == nil {
		return nil, fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryGetEntry(id)
}

// MemoryListEntries 全局逐条列举条目（跨全部集合）
func MemoryListEntries(mediaType string, offset, limit int) ([]MemoryQueryResult, int) {
	if globalMemoryDB == nil {
		return nil, 0
	}
	return globalMemoryDB.MemoryListEntries(mediaType, offset, limit)
}

// MemoryCountEntries 全局统计指定类型的条目数
func MemoryCountEntries(mediaType string) int {
	if globalMemoryDB == nil {
		return 0
	}
	return globalMemoryDB.MemoryCountEntries(mediaType)
}

// MemoryDeleteMessage 全局删除消息（跨全部集合）
func MemoryDeleteMessage(ctx context.Context, id string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryDeleteMessage(ctx, id)
}

// MemoryGetCollectionCount 全局获取集合
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

// MemoryReloadCollection 全局重新加载分组（外部直接改写分片文件后刷新内存缓存）
func MemoryReloadCollection(collectionName string) error {
	if globalMemoryDB == nil {
		return fmt.Errorf("全局 MemoryDB 未初始化")
	}
	return globalMemoryDB.MemoryReloadCollection(collectionName)
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
