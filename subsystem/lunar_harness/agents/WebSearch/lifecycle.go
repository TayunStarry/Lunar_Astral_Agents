package WebSearch

// lifecycle.go 模型生命周期：步骤 0 卸载 system-multimodal、加载 system-search，
// 收尾时卸载 system-search（主模型交给 router 在月华下一次请求时按需重载）。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
)

// prepareSearchModel 执行步骤 0 的模型切换：卸载主多模态模型并加载搜索模型。
// 切换是「尽力而为」：路由不可达或搜索模型加载失败时只告警并退回主模型，
// 不让整个搜索因为模型切换失败而无法启动。
func prepareSearchModel(cfg Config) (summaryProfile, func()) {
	fallback := mainSummaryProfile(cfg)
	if !cfg.swapEnabled() {
		return fallback, func() {}
	}
	if _, err := llama.UnloadModelByID(config.AgentMultimodalModel); err != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "卸载多模态模型[%s]失败，继续加载搜索模型: %v", config.AgentMultimodalModel, err)
	} else {
		logger.Info("CrystalAstral", "Web-Search 已卸载多模态模型[%s]", config.AgentMultimodalModel)
	}

	if err := warmUpSearchModel(cfg.SearchModel); err != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "搜索模型[%s]不可用，本次改用主模型继续搜索: %v", cfg.SearchModel, err)
		return fallback, func() {}
	}
	logger.Info("CrystalAstral", "Web-Search 已加载搜索模型[%s]", cfg.SearchModel)

	return searchSummaryProfile(cfg), func() {
		if _, err := llama.UnloadModelByID(cfg.SearchModel); err != nil {
			logger.SubWarn("CrystalAstral", "WebSearch", "收尾卸载搜索模型[%s]失败: %v", cfg.SearchModel, err)
			return
		}
		logger.Info("CrystalAstral", "Web-Search 已卸载搜索模型[%s]", cfg.SearchModel)
	}
}

// mainSummaryProfile 主模型摘要档位：按配置分块，多段时再做一次合并
func mainSummaryProfile(cfg Config) summaryProfile {
	return summaryProfile{
		ChunkChars: cfg.TextChunkChars,
		MaxChunks:  cfg.TextMaxChunks,
		Merge:      true,
	}
}

// searchSummaryProfile 搜索小模型摘要档位：整页大块、多段直接拼接、不再调用合并模型
func searchSummaryProfile(cfg Config) summaryProfile {
	return summaryProfile{
		Model:      cfg.SearchModel,
		ChunkChars: 16000,
		MaxChunks:  2,
		Merge:      false,
	}
}
