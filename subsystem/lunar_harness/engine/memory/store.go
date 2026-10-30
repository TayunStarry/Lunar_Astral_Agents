package memory

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// newStore 创建记忆库数据库实例（内部使用）
func newStore(baseDir string) *Store {
	return &Store{
		baseDir:     baseDir,
		collections: make(map[string]*Collection),
	}
}

// currentEmbeddingModel 当前记忆库使用的嵌入模型名。
//
// 新架构不再有独立的记忆库模型配置：记忆库与智能体共用 agent.embedding_*，
// 因此这里统一读取 AgentEmbeddingModel。
func currentEmbeddingModel() string {
	if model := strings.TrimSpace(config.AgentEmbeddingModel); model != "" {
		return model
	}
	return "text_embedding"
}

// EnsureReady 初始化记忆库实例：一次性加载全部集合，并补齐默认分组与集合名向量。
//
// 启动时不再指定某个集合：磁盘上的所有集合全部载入，默认分组集合缺失时自动创建，
// 没有集合名嵌入向量的集合按集合名调用嵌入模型补齐。
// 模型配置为本地固定值，与智能体共用嵌入/多模态模型。
func (d *Store) EnsureReady() error {
	if d.baseDir == "" {
		return fmt.Errorf("记忆库 baseDir 为空，请先调用 Init")
	}
	if err := os.MkdirAll(d.baseDir, 0755); err != nil {
		return fmt.Errorf("创建记忆库目录失败: %w", err)
	}

	d.initMu.Lock()
	defer d.initMu.Unlock()

	if !d.initialized {
		d.httpClient = &http.Client{Timeout: 120 * time.Second}
		d.initialized = true
		d.loadAllCollections()
	}

	// 每次实例初始化都做一遍幂等补齐：默认分组、缺失的集合名向量
	// 嵌入服务启动较晚时，后续 EnsureReady 调用会在这里继续补齐，而不是永久停摆
	defaultsOK := d.ensureDefaultCollections()
	namesOK := d.ensureNameEmbeddings()

	if d.CollectionCount() == 0 {
		return fmt.Errorf("记忆库没有可用集合（默认分组创建失败，请确认嵌入服务可用后重试）")
	}
	if !defaultsOK || !namesOK {
		return fmt.Errorf("记忆库分组尚未全部就绪（嵌入服务可能未启动，稍后重试）")
	}
	return nil
}

// ensureDefaultCollections 确保默认分组集合存在（已存在则跳过），返回是否全部就绪
func (d *Store) ensureDefaultCollections() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ok := true
	for _, name := range DefaultCollections {
		d.collectionsMu.RLock()
		_, exists := d.collections[name]
		d.collectionsMu.RUnlock()
		if exists {
			continue
		}
		if err := d.InitCollection(ctx, name); err != nil {
			logger.Warn("FileManager", "默认记忆分组 [%s] 初始化失败: %v（下次启动重试）", name, err)
			ok = false
			continue
		}
		logger.Info("FileManager", "默认记忆分组 [%s] 已就绪", name)
	}
	return ok
}

// ensureNameEmbeddings 为所有缺少集合名嵌入向量的集合补齐向量，返回是否全部就绪
func (d *Store) ensureNameEmbeddings() bool {
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
		ready := len(c.NameEmbedding) > 0 && (c.NameEmbeddingModel == "" || c.NameEmbeddingModel == currentEmbeddingModel())
		c.mu.RUnlock()
		if ready {
			continue
		}
		if err := d.ensureCollectionNameEmbedding(ctx, c); err != nil {
			logger.Warn("FileManager", "集合 [%s] 名称嵌入生成失败: %v（下次启动重试）", c.Name, err)
			ok = false
			continue
		}
		logger.Info("FileManager", "集合 [%s] 名称嵌入向量已生成（模型 %s）", c.Name, c.NameEmbeddingModel)
	}
	return ok
}

// ensureCollectionNameEmbedding 为单个集合补齐名称嵌入向量
func (d *Store) ensureCollectionNameEmbedding(ctx context.Context, c *Collection) error {
	c.mu.RLock()
	ready := len(c.NameEmbedding) > 0 && (c.NameEmbeddingModel == "" || c.NameEmbeddingModel == currentEmbeddingModel())
	name := c.Name
	c.mu.RUnlock()
	if ready {
		return nil
	}
	model := currentEmbeddingModel()
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

// Ready 检查记忆库是否已初始化
func (d *Store) Ready() bool {
	return d.initialized
}
