package memory

import (
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

// EnsureReady 初始化记忆库实例：按 clusters.json 加载簇目录，必要时执行幂等布局迁移。
//
// 布局与索引一致时只加载，不搬迁；存在旧分组目录时先备份，再按簇搬迁、逐项对账，
// 全部成功后才切换索引并删除旧目录。任一步失败回滚，不改变旧布局。
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
	}

	// 聚类索引与磁盘布局统一由 prepareLayout 准备（幂等，可重复调用）。
	if _, err := d.prepareLayout(false); err != nil {
		logger.Warn("FileManager", "聚类布局初始化失败（旧目录保持不变）: %v", err)
		// 记录失败原因：此时集合可能完全没加载，读取方必须能看出「读不到」而不是「没有数据」。
		d.recordLoadError(fmt.Errorf("记忆库聚类布局初始化失败: %w", err))
		return fmt.Errorf("记忆库聚类布局初始化失败: %w", err)
	}

	// 关键词倒排索引：优先加载持久化快照，缺失/损坏/与文档不一致时从已加载文档幂等重建。
	// 索引失败不阻断初始化，检索时降级为向量通道。
	d.ensureKeywordIndex()
	return nil
}

// Ready 检查记忆库是否已初始化
func (d *Store) Ready() bool {
	return d.initialized
}
