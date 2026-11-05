package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// clusterDirName 返回 ID 对应的目录名：
//   - 总库（StoreCollectionID = "store-all"）固定返回 "store-all"，直观且不用 hash
//   - 正式簇返回 c-<sha256(簇 ID) 前 8 位十六进制>，同一簇 ID 永远同一目录名
func clusterDirName(id string) string {
	if id == StoreCollectionID {
		return StoreCollectionID
	}
	sum := sha256.Sum256([]byte(id))
	return ClusterDirPrefix + hex.EncodeToString(sum[:])[:ClusterDirHashLen]
}

// isClusterDirName 判断目录名是否为正式簇目录（c-<8 位十六进制>）
func isClusterDirName(name string) bool {
	if !strings.HasPrefix(name, ClusterDirPrefix) {
		return false
	}
	rest := strings.TrimPrefix(name, ClusterDirPrefix)
	if len(rest) != ClusterDirHashLen {
		return false
	}
	for _, r := range rest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// isStoreDirName 判断目录名是否为总库目录
func isStoreDirName(name string) bool {
	return name == StoreCollectionID
}

// storeCollection 获取或创建总库的物理存储单元（首次写入时才落目录）。
// 总库固定 ID = "store-all"，固定目录 = "store-all"，不做 hash。
func (d *Store) storeCollection() (*Collection, error) {
	return d.collectionForCluster(StoreCollectionID, true)
}

// clusterForCollection 获取或创建正式簇的物理存储单元（簇 ID 不允许等于总库 ID）。
func (d *Store) clusterForCollection(clusterID string) (*Collection, error) {
	if clusterID == StoreCollectionID {
		return nil, fmt.Errorf("不能用正式簇 ID 创建总库目录")
	}
	return d.collectionForCluster(clusterID, false)
}

// collectionForID 按任意 ID 获取存储单元（总库或簇）。
func (d *Store) collectionForID(id string) (*Collection, error) {
	return d.collectionForCluster(id, id == StoreCollectionID)
}

// collectionForCluster 获取或创建存储单元（总库或簇）。
// isStore 为 true 时强制 Kind = "store"。
func (d *Store) collectionForCluster(clusterID string, isStore bool) (*Collection, error) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, fmt.Errorf("存储单元 ID 不能为空")
	}
	d.collectionsMu.RLock()
	existing, ok := d.collections[clusterID]
	d.collectionsMu.RUnlock()
	if ok {
		return existing, nil
	}

	collDir := filepath.Join(d.baseDir, clusterDirName(clusterID))
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return nil, fmt.Errorf("创建存储单元目录失败: %w", err)
	}
	dim := 0
	d.clusterMu.RLock()
	if d.clusterIndex != nil {
		dim = d.clusterIndex.Dimension
	}
	d.clusterMu.RUnlock()

	kind := "cluster"
	if isStore {
		kind = "store"
	}

	c := &Collection{
		Name:      clusterID,
		Kind:      kind,
		Dimension: dim,
		Documents: make([]Document, 0),
		collDir:   collDir,
		metaPath:  filepath.Join(collDir, "metadata.json"),
	}
	if err := c.saveCollectionMeta(); err != nil {
		return nil, fmt.Errorf("保存元数据失败: %w", err)
	}
	c.updateLastModTime()

	d.collectionsMu.Lock()
	d.collections[clusterID] = c
	d.collectionsMu.Unlock()
	return c, nil
}

// clearCollectionDir 清空簇目录内旧数据文件（不做旧版数据迁移，直接废弃）
func clearCollectionDir(collDir string) {
	patterns := []string{"documents_*.json", "images_*.json", "tags_*.json", "contents_*.json", "embeddings_*.json", "base64_*.json", "documents.json", "metadata.json"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(collDir, pattern))
		for _, f := range files {
			os.Remove(f)
		}
	}
}

// getCollection 获取簇存储单元（线程安全）
func (d *Store) getCollection(name string) (*Collection, error) {
	d.collectionsMu.RLock()
	c, ok := d.collections[name]
	d.collectionsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("簇 '%s' 不存在，请先初始化", name)
	}
	return c, nil
}

// snapshotCollections 返回全部簇存储单元的快照（用于全局检索与聚类）
func (d *Store) snapshotCollections() []*Collection {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	collections := make([]*Collection, 0, len(d.collections))
	for _, c := range d.collections {
		collections = append(collections, c)
	}
	// 固定顺序：相似度并列时按簇 ID 兜底，保证结果可复现
	sort.Slice(collections, func(i, j int) bool { return collections[i].Name < collections[j].Name })
	return collections
}

// loadClusterCollections 按 clusters.json 加载存储单元：
// 先加载总库（store-all），再加载索引中的正式簇（c-*）。
// 不在索引中的 c-* 目录不会被加载（迁移成功后由迁移流程清理）。
func (d *Store) loadClusterCollections(index *ClusterIndex) error {
	d.collectionsMu.Lock()
	d.collections = make(map[string]*Collection)
	d.loadErr = nil
	d.collectionsMu.Unlock()
	if d.baseDir == "" {
		return nil
	}

	// 1. 加载总库
	d.loadSingleCollection(StoreCollectionID, "store")

	// 2. 加载正式簇
	if index != nil {
		for _, node := range index.Clusters {
			d.loadSingleCollection(node.ID, "cluster")
		}
	}

	logger.Info("FileManager", "已加载总库 + %d 个正式簇目录", len(d.collections)-1)
	return nil
}

// loadSingleCollection 加载单个存储单元（总库或正式簇）。不存在时跳过。
func (d *Store) loadSingleCollection(id, kind string) {
	dir := filepath.Join(d.baseDir, clusterDirName(id))
	metaPath := filepath.Join(dir, "metadata.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return
	}
	var meta collectionMeta
	if err := readJSONFile(metaPath, &meta); err != nil {
		logger.Warn("FileManager", "%s 目录 %s 不可读，跳过加载: %v", kind, dir, err)
		d.recordLoadError(fmt.Errorf("%s 目录 %s 不可读: %w", kind, dir, err))
		return
	}
	c := &Collection{
		Name:      id,
		Kind:      kind,
		Dimension: meta.EmbeddingDimension,
		collDir:   dir,
		metaPath:  metaPath,
	}
	if err := c.loadDocumentsFromFile(); err != nil {
		logger.Error("FileManager", "加载 %s %s 文档失败: %v", kind, id, err)
		d.recordLoadError(fmt.Errorf("加载 %s %s 文档失败: %w", kind, id, err))
		return
	}
	c.updateLastModTime()

	d.collectionsMu.Lock()
	d.collections[id] = c
	d.collectionsMu.Unlock()
}

// recordLoadError 记录启动加载的首个失败原因（只留第一条，避免掩盖根因）。
func (d *Store) recordLoadError(err error) {
	if err == nil {
		return
	}
	d.collectionsMu.Lock()
	if d.loadErr == nil {
		d.loadErr = err
	}
	d.collectionsMu.Unlock()
}

// loadError 返回启动加载失败原因；nil 表示加载正常。
func (d *Store) loadError() error {
	d.collectionsMu.RLock()
	defer d.collectionsMu.RUnlock()
	return d.loadErr
}

// readDocumentsFromDir 读取任意目录（旧分组目录 / 簇目录）的文档与向量分片。
// 返回文档（已挂回向量并补齐类型）、文档分块数、向量分块数；目录不存在时返回空。
func readDocumentsFromDir(dir string) ([]Document, int, int, error) {
	var allDocs []Document
	docChunks := 0
	for i := 1; ; i++ {
		path := filepath.Join(dir, fmt.Sprintf("documents_%04d.json", i))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		var docs []Document
		if err := readJSONFile(path, &docs); err != nil {
			return nil, 0, 0, fmt.Errorf("读取 %s 失败: %w", path, err)
		}
		allDocs = append(allDocs, docs...)
		docChunks = i
	}

	embMap := make(map[string][]float32, len(allDocs))
	embChunks := 0
	for i := 1; ; i++ {
		path := filepath.Join(dir, fmt.Sprintf("embeddings_%04d.json", i))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		var records []embeddingRecord
		if err := readJSONFile(path, &records); err != nil {
			return nil, 0, 0, fmt.Errorf("读取 %s 失败: %w", path, err)
		}
		for _, rec := range records {
			embMap[rec.ID] = rec.Embedding
		}
		embChunks = i
	}

	for j := range allDocs {
		if vec, ok := embMap[allDocs[j].ID]; ok {
			allDocs[j].Embedding = vec
		}
		// 旧数据没有 type 字段：按内容推断并回写，保证后续类型化查询可用
		if allDocs[j].Type == "" {
			allDocs[j].Type = documentType(allDocs[j])
		}
	}
	return allDocs, docChunks, embChunks, nil
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
// 持久化 — 元数据、文档
// =============================================================================

// saveCollectionMeta 保存集合元数据到 metadata.json
func (c *Collection) saveCollectionMeta() error {
	c.mu.RLock()
	meta := collectionMeta{
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

// loadDocumentsFromFile 从分块文件加载文档与嵌入向量到内存
// documents_NNNN.json 存文档本体（不含向量），embeddings_NNNN.json 存向量并以 ID 关联
func (c *Collection) loadDocumentsFromFile() error {
	docs, docChunks, embChunks, err := readDocumentsFromDir(c.collDir)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.Documents = docs
	c.documentsChunkCount = docChunks
	c.embeddingsChunkCount = embChunks
	c.rebuildPositionsLocked()
	c.mu.Unlock()
	return nil
}

// rebuildPositionsLocked 重建「条目 ID → 下标」映射（调用方须持有写锁）。
func (c *Collection) rebuildPositionsLocked() {
	c.positions = make(map[string]int, len(c.Documents))
	for i := range c.Documents {
		if id := c.Documents[i].ID; id != "" {
			c.positions[id] = i
		}
	}
}

// documentByID 按 ID 取文档副本；倒排索引召回后用它 O(1) 定位正文。
func (c *Collection) documentByID(id string) (Document, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if i, ok := c.positions[id]; ok && i >= 0 && i < len(c.Documents) && c.Documents[i].ID == id {
		return c.Documents[i], true
	}
	// 兼容旧路径/测试直接构造的集合：位置表可能尚未建立，退化为线性查找
	for i := range c.Documents {
		if c.Documents[i].ID == id {
			return c.Documents[i], true
		}
	}
	return Document{}, false
}

// saveDocumentsToFile 将文档与嵌入向量保存到分块文件（100 条一块）
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

	c.mu.Lock()
	c.documentsChunkCount = newChunkCount
	c.embeddingsChunkCount = newChunkCount
	c.mu.Unlock()

	return c.saveCollectionMeta()
}

// reloadIfChanged 检查 metadata.json 是否被外部进程修改，若是则重新加载
func (c *Collection) reloadIfChanged() {
	if info, err := os.Stat(c.metaPath); err != nil {
		return
	} else if !info.ModTime().Equal(c.lastFileModTime) {
		c.loadDocumentsFromFile()
		c.updateLastModTime()
	}
}
