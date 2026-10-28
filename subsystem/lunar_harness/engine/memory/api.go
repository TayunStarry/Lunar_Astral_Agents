package memory

import (
	"context"
	"fmt"
)

// =============================================================================
// 全局包装函数 — 供外部模块调用
//
// 各智能体只依赖本包：全局实例由 Init 建立，其余函数统一委托给该实例。
// =============================================================================

// Init 全局初始化（幂等：若已初始化则直接返回现有实例）
func Init(baseDir string) *Store {
	if globalStore != nil {
		return globalStore
	}
	globalStore = newStore(baseDir)
	return globalStore
}

// Current 获取全局记忆库实例
func Current() *Store {
	return globalStore
}

// Ready 全局检查记忆库初始化状态
func Ready() bool {
	if globalStore == nil {
		return false
	}
	return globalStore.Ready()
}

// EnsureReady 全局初始化记忆库实例（模型配置从 agent 分组读取）
func EnsureReady() error {
	if globalStore == nil {
		return fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.EnsureReady()
}

// InitCollection 全局初始化集合（创建时按集合名生成嵌入向量）
func InitCollection(ctx context.Context, name string) error {
	if globalStore == nil {
		return fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.InitCollection(ctx, name)
}

// AddText 全局添加文本消息（内容自动归组）
func AddText(ctx context.Context, role, content string) (string, error) {
	if globalStore == nil {
		return "", fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.AddText(ctx, role, content)
}

// AddTextWithTags 全局添加文本消息（携带预设标签与来源标记）
func AddTextWithTags(ctx context.Context, role, content string, tags []string, source string) (string, error) {
	if globalStore == nil {
		return "", fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.AddTextWithTags(ctx, role, content, tags, source)
}

// AddImage 全局添加图片（内容自动归组）
func AddImage(ctx context.Context, base64Image string, orientation string, custom string) (string, error) {
	if globalStore == nil {
		return "", fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.AddImage(ctx, base64Image, orientation, custom)
}

// AddAudio 全局添加音频（按描述文本归组）
func AddAudio(ctx context.Context, base64Audio string, description string) (string, error) {
	if globalStore == nil {
		return "", fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.AddAudio(ctx, base64Audio, description)
}

// Query 全局按类型查询（跨全部集合；选项控制检索范围）
func Query(ctx context.Context, queryText, mediaType string, topK int, opts QueryOptions) ([]Entry, error) {
	if globalStore == nil {
		return nil, fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.Query(ctx, queryText, mediaType, topK, opts)
}

// KeywordSearch 全局关键词/正则检索（不调用嵌入服务）
func KeywordSearch(mediaType string, terms []string, useRegex bool, limit int) ([]Entry, error) {
	if globalStore == nil {
		return nil, fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.KeywordSearch(mediaType, terms, useRegex, limit)
}

// Update 全局原地更新文本记忆正文
func Update(ctx context.Context, id, content string) error {
	if globalStore == nil {
		return fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.Update(ctx, id, content)
}

// Delete 全局删除消息（跨全部集合）
func Delete(ctx context.Context, id string) error {
	if globalStore == nil {
		return fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.Delete(ctx, id)
}

// Fuse 全局权重融合：对指定集合的文本记忆执行合并
func Fuse(ctx context.Context, collectionName string, ids []string, opts FuseOptions) (FuseResult, error) {
	if globalStore == nil {
		return FuseResult{}, fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.Fuse(ctx, collectionName, ids, opts)
}

// Rebuild 全局重建集合内容向量
func Rebuild(ctx context.Context, collectionName, modelName string, progress func(int, int)) error {
	if globalStore == nil {
		return fmt.Errorf("全局记忆库未初始化")
	}
	return globalStore.Rebuild(ctx, collectionName, modelName, progress)
}
