package lunar_chromedp

import (
	"fmt"
	"strings"
)

// =============================================================================
// 配置管理 — 默认值与校验
// =============================================================================

// DefaultSearchConfig 返回搜索智能体的默认配置
// 调用方可基于此配置修改后传入 InitSearch
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		MultimodalURL:   "http://127.0.0.1:8080/v1",
		MultimodalName:  "system-multimodal",
		MultimodalKey:   "",
		EmbeddingURL:    "http://127.0.0.1:8080/v1",
		EmbeddingName:   "system-embedding",
		EmbeddingKey:    "",
		MaxContextTokens: MaxContextTokensDefault,
	}
}

// ValidateSearchConfig 校验搜索配置的有效性
// 返回 nil 表示配置有效，否则返回描述性错误
func ValidateSearchConfig(config SearchConfig) error {
	if strings.TrimSpace(config.MultimodalURL) == "" {
		return fmt.Errorf("多模态模型 URL 不能为空")
	}
	if strings.TrimSpace(config.MultimodalName) == "" {
		return fmt.Errorf("多模态模型名称不能为空")
	}
	if strings.TrimSpace(config.EmbeddingURL) == "" {
		return fmt.Errorf("嵌入模型 URL 不能为空")
	}
	if strings.TrimSpace(config.EmbeddingName) == "" {
		return fmt.Errorf("嵌入模型名称不能为空")
	}
	return nil
}
