package search

// lifecycle.go 模型生命周期：步骤 0 卸载 primary_reasoning、改用 light_reasoning
// （由 router 在首个请求时加载），收尾时卸载 light_reasoning（主模型交给 router
// 在月华下一次请求时按需重载）。

import (
	"LunarSubsystem/LunarHarness/chain/search/crawler"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/modellease"
)

// prepareSearchModel 执行步骤 0 的模型切换：卸载主多模态模型并改用搜索模型。
// 返回的 model 交给规划/摘要智能体；返回的函数在收尾时卸载搜索模型并释放租约。
func prepareSearchModel(cfg crawler.Config) (model string, unload func()) {
	// 与文献解析的轻量扫描通道共用同一把切换锁，避免两个智能体同时卸载/加载模型
	releaseLease, _, err := modellease.AcquireUnload(config.AgentMultimodalModel)
	if err != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "卸载多模态模型[%s]失败，继续尝试搜索模型: %v", config.AgentMultimodalModel, err)
	} else {
		logger.Info("CrystalAstral", "Web-Search 已卸载多模态模型[%s]", config.AgentMultimodalModel)
	}
	logger.Info("CrystalAstral", "Web-Search 本次使用搜索模型[%s]", cfg.SearchModel)

	return cfg.SearchModel, func() {
		defer releaseLease()
		if _, err := modellease.UnloadHeld(cfg.SearchModel); err != nil {
			logger.SubWarn("CrystalAstral", "WebSearch", "收尾卸载搜索模型[%s]失败: %v", cfg.SearchModel, err)
			return
		}
		logger.Info("CrystalAstral", "Web-Search 已卸载搜索模型[%s]", cfg.SearchModel)
	}
}
