package WebSearch

// lifecycle.go 模型生命周期：步骤 0 卸载 primary_reasoning、改用 light_reasoning
// （由 router 在首个请求时加载），收尾时卸载 light_reasoning（主模型交给 router
// 在月华下一次请求时按需重载）。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
)

// prepareSearchModel 执行步骤 0 的模型切换：卸载主多模态模型并改用搜索模型。
// 不做预热探测：搜索模型能否加载由 router 在首个请求时给出（不可用即 404），
// 该错误交给检索词提炼与摘要的既有降级链兜底。
func prepareSearchModel(cfg Config) (summaryProfile, func()) {
	if !cfg.swapEnabled() {
		return mainSummaryProfile(cfg), func() {}
	}
	if _, err := llama.UnloadModelByID(config.AgentMultimodalModel); err != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "卸载多模态模型[%s]失败，继续尝试搜索模型: %v", config.AgentMultimodalModel, err)
	} else {
		logger.Info("CrystalAstral", "Web-Search 已卸载多模态模型[%s]", config.AgentMultimodalModel)
	}
	logger.Info("CrystalAstral", "Web-Search 本次使用搜索模型[%s]", cfg.SearchModel)

	return searchSummaryProfile(cfg), func() {
		if _, err := llama.UnloadModelByID(cfg.SearchModel); err != nil {
			logger.SubWarn("CrystalAstral", "WebSearch", "收尾卸载搜索模型[%s]失败: %v", cfg.SearchModel, err)
			return
		}
		logger.Info("CrystalAstral", "Web-Search 已卸载搜索模型[%s]", cfg.SearchModel)
	}
}

// mainSummaryProfile 主模型摘要档位
func mainSummaryProfile(cfg Config) summaryProfile {
	return summaryProfile{}
}

// searchSummaryProfile 搜索小模型摘要档位
func searchSummaryProfile(cfg Config) summaryProfile {
	return summaryProfile{Model: cfg.SearchModel}
}
