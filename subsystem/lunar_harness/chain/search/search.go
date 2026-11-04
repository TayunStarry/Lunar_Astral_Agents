package search

// search.go 搜索链入口：规划 → 抓取 → 摘要 三段式调度。
//
//	步骤 0：卸载主多模态模型 → 加载搜索模型（链侧持有模型切换租约）
//	步骤 1：调度「搜索者-规划」产出结构化决策（模型调用 1/2）；命中天气直连则直接返回
//	步骤 2：调度执行体「SearchCrawler」完成引擎降级、详情页采集（不发起模型调用）
//	步骤 3：调度「搜索者-摘要」把全部正文一次提取为针对性摘要（模型调用 2/2）
//	步骤 4：摘要即最终输出；仅失败时走确定性兜底
//	步骤 5：卸载搜索模型（主模型交给 router 按需重载），返回最终输出
//
// 本包只做调度、上下文管理与循环控制，不内嵌任何模型调用实例。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/SearchPlanner"
	"LunarSubsystem/LunarHarness/agents/SearchSummarizer"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/weather"
	"LunarSubsystem/LunarHarness/executors/SearchCrawler"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Run 执行一次网络搜索：月华自然语言指令 → 搜索报告（文本）。
// 失败统一上报给智能修复（循环截断与主动取消已在 ReportRun 内过滤）。
func Run(instruction string) (out string, err error) {
	defer func() { agentbase.ReportRun("web_search", err) }()
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", fmt.Errorf("空指令")
	}

	runMu.Lock()
	defer runMu.Unlock()

	cfg := SearchCrawler.LoadConfig()
	started := time.Now()
	deadline := started.Add(time.Duration(cfg.MaxRunSeconds) * time.Second)
	runCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	logger.Info("CrystalAstral", "Web-Search 网络搜索开始: %s", SearchCrawler.TruncateRunes(instruction, 512))

	// 步骤 0 + 1：模型切换后调度规划智能体；决策失败时按其兜底决策继续（行为等价）
	model, unloadSearchModel := prepareSearchModel(cfg)
	defer unloadSearchModel()
	decision, planErr := SearchPlanner.Plan(runCtx, instruction, model)
	if planErr != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "%v", planErr)
	}
	logger.Info("CrystalAstral", "Web-Search 路由决策: pipeline=%s 把握=%.2f query=%s",
		decision.Pipeline, decisionConfidence(decision), SearchCrawler.TruncateRunes(decision.Query, 512))

	// 直连管线：命中后直接返回结果，不进搜索引擎与浏览器
	if decision.Pipeline == SearchPlanner.PipelineWeather {
		ctx, cancelWeather := context.WithTimeout(context.Background(), weather.QueryTimeout)
		data, weatherErr := weather.Query(ctx, SearchPlanner.WeatherCity(decision))
		cancelWeather()
		if weatherErr == nil {
			logger.Info("CrystalAstral", "Web-Search 天气直连完成（跳过检索）: %s", data.City)
			return weather.Render(data), nil
		}
		logger.SubWarn("CrystalAstral", "WebSearch", "天气查询失败，回退检索管线: %v", weatherErr)
		decision.Query = instruction
	}
	query := decision.Query

	// 步骤 2：调度抓取执行体（指令中的「前N个」优先于配置的条目余量）
	n := cfg.DetailCount()
	if m := SearchCrawler.PageCountFromInstruction(instruction); m > 0 {
		n = m
	}
	crawl, crawlErr := SearchCrawler.Crawl(query, n, cfg, deadline)
	if crawlErr != nil {
		return "", crawlErr
	}
	logger.Info("CrystalAstral", "Web-Search 使用引擎 [%s]: %d 条结果", crawl.Engine, len(crawl.Extract.Results))

	// 步骤 3：调度摘要智能体（以原始指令为准绳剔除噪音）
	digest, digestErr := SearchSummarizer.Digest(runCtx, instruction, query, buildSources(crawl), model)
	if digestErr != nil {
		logger.SubWarn("CrystalAstral", "WebSearch", "摘要提取失败，输出将走确定性兜底: %v", digestErr)
	}

	// 步骤 4：摘要即最终输出（失败时兜底）
	in := reportInput{
		Instruction:    instruction,
		Query:          query,
		EngineName:     crawl.EngineName,
		Engine:         crawl.Extract,
		SearchPageText: crawl.SearchText,
		Digest:         digest,
		Pages:          crawl.Pages,
		Interrupted:    crawl.Interrupted,
		Elapsed:        time.Since(started).Truncate(time.Second).String(),
	}
	report := composeReport(in)
	recordLastSearch(query, crawl.Engine, report, len(crawl.Pages), started)

	// 步骤 5：返回前由 defer 卸载搜索模型，随后把最终输出交给月华
	logger.Info("CrystalAstral", "Web-Search 网络搜索完成: 引擎=%s, 检索词=%s, 页面=%d/%d, 中断=%v, 耗时=%s",
		crawl.Engine, query, len(crawl.Pages), n, crawl.Interrupted, time.Since(started).Truncate(time.Second))
	return report, nil
}

// buildSources 组装摘要来源：搜索结果页在前，详情页按打开顺序在后
func buildSources(crawl SearchCrawler.CrawlResult) []SearchSummarizer.Source {
	sources := make([]SearchSummarizer.Source, 0, len(crawl.Pages)+1)
	if strings.TrimSpace(crawl.SearchText) != "" {
		sources = append(sources, SearchSummarizer.Source{
			Label: "搜索结果页",
			Title: "搜索结果页（首页提取）",
			Text:  crawl.SearchText,
		})
	}
	for i, cap := range crawl.Pages {
		sources = append(sources, SearchSummarizer.Source{
			Label: fmt.Sprintf("详情页 %d", i+1),
			Title: SearchCrawler.FirstNonEmpty(cap.Title, cap.Domain),
			URL:   cap.URL,
			Text:  cap.Text,
		})
	}
	return sources
}

// decisionConfidence 读取规划决策的把握（未给出时按「可信」处理）
func decisionConfidence(decision SearchPlanner.Decision) float64 {
	if decision.Confidence == nil {
		return 1
	}
	return *decision.Confidence
}
