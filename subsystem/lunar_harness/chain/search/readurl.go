package search

// readurl.go 指定页面读取（用户直接发来、还没有缓存摘要的网页地址）
//
// 与搜索链共用同一套模型生命周期：先卸载主多模态模型，改用搜索模型
// 打开页面并提炼摘要，完成后卸载搜索模型（主模型由 router 在月华下一次
// 请求时按需重载）。
//
// 提炼结果写入统一句柄库的 URL 主表（parse_text），
// 资源目录据此给句柄直接返回「地址 + 标题 + 解析文本」，月华下次看到同一地址不再开页面。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/SearchSummarizer"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/executors/WebCrawler"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// ReadURL 打开指定页面并提炼摘要：返回页面标题与摘要
//
// instruction 是月华对这次引用的具体诉求（可空，交给默认提炼指令）。
func ReadURL(target, instruction string) (title, summary string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", fmt.Errorf("页面地址为空")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		instruction = readURLInstruction
	}

	runMu.Lock()
	defer runMu.Unlock()

	cfg := WebCrawler.LoadConfig()
	// 单页读取使用独立预算：不复用整轮多页检索的 MaxRunSeconds。
	budget := cfg.MaxRunSeconds
	if budget <= 0 || budget > readPageMaxSeconds {
		budget = readPageMaxSeconds
	}
	deadline := time.Now().Add(time.Duration(budget) * time.Second)
	runCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	logger.Info("CrystalAstral", "Web-Search 打开指定页面: %s", textutil.TruncateRunes(target, 512))

	// 模型生命周期与网络搜索一致：卸主模型 → 搜索模型提炼 → 收尾卸搜索模型
	model, unloadSearchModel := prepareSearchModel(cfg)
	defer unloadSearchModel()

	cap, fetchErr := WebCrawler.FetchPage(target, budget, cfg, deadline)
	if fetchErr != nil {
		return "", "", fetchErr
	}

	title = textutil.FirstNonEmpty(cap.Title, WebCrawler.DomainOf(target), target)
	summary, err = SearchSummarizer.Digest(runCtx, instruction, title, []SearchSummarizer.Source{{
		Label: "指定页面",
		Title: title,
		URL:   cap.URL,
		Text:  cap.Text,
	}}, model)
	summary = strings.TrimSpace(summary)
	if err != nil || summary == "" {
		if err == nil {
			err = fmt.Errorf("页面摘要为空")
		}
		return title, "", err
	}

	// 记入近期访问缓存：同一地址下次直接命中，不再重复开页面
	if cacheErr := WebCrawler.RememberSummary(target, title, summary, cfg); cacheErr != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "近期访问缓存不可用，本次结果不回填: %v", cacheErr)
	}
	logger.Info("CrystalAstral", "Web-Search 指定页面提炼完成: %s", textutil.TruncateRunes(target, 512))
	return title, summary, nil
}
