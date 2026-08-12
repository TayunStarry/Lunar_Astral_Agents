package AgentSearch

import "fmt"

// =============================================================================
// 配置管理 — 默认值与校验
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取
// =============================================================================

// DefaultSearchConfig 返回搜索智能体的默认配置
// 调用方可基于此配置修改后传入 InitSearch
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		MemoryDBDir:      "local_data/database/memory",
		MaxContextTokens: MaxContextTokensDefault,
	}
}

// ValidateSearchConfig 校验搜索配置的有效性
// 返回 nil 表示配置有效，否则返回描述性错误
func ValidateSearchConfig(config SearchConfig) error {
	if config.MaxContextTokens <= 0 {
		return fmt.Errorf("最大上下文 tokens 必须大于 0")
	}
	return nil
}
