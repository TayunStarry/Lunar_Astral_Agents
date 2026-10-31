package WebSearch

// Web-Search · 网络搜索内置智能体（web_search）
//
//	步骤 0：卸载 primary_reasoning → 加载 light_reasoning → 同一次模型调用产出结构化决策
//	         （pipeline + 检索词/参数 + 把握）；命中直连管线则直接返回结果
//	步骤 1：打开「网页搜索:首页」，搜索引擎降级链 + 完整滚动 + 结果与正文提取，随后关闭首页
//	步骤 2：从结果中筛选出需打开的详情页链接（去重/近期已访问过滤/数量上限）
//	步骤 3：用「网页搜索:详情」同时打开全部详情页，全部等待加载后统一滚动提取正文，随后统一关窗
//	步骤 4：把全部页面正文拼接为长文，连同提示词一口气交给模型做针对性摘要提取（模型调用 2/2）
//	步骤 5：摘要提取结果即最终输出；仅失败时走确定性兜底
//	步骤 6：卸载 light_reasoning（主模型交给 router 按需重载），返回最终输出
//
// 全程只使用文本，不依赖任何截图或视觉识别路径；业务完成后关闭浏览器页面。
// 配置来自 engine/config 统一解析的 web_search 节。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/weather"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Run 执行一次网络搜索：月华自然语言指令 → 搜索报告（文本）。
func Run(instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", fmt.Errorf("空指令")
	}

	runMu.Lock()
	defer runMu.Unlock()

	cfg := LoadConfig()
	started := time.Now()
	deadline := started.Add(time.Duration(cfg.MaxRunSeconds) * time.Second)
	logger.Info("CrystalAstral", "Web-Search 网络搜索开始: %s", truncateRunes(instruction, 80))

	// 步骤 0：卸载多模态模型、加载搜索模型，并在同一次调用里决定管线与检索词
	profile, unloadSearchModel := prepareSearchModel(cfg)
	defer unloadSearchModel()
	decision := Route(instruction, profile)
	logger.Info("CrystalAstral", "Web-Search 路由决策: pipeline=%s 把握=%.2f query=%s",
		decision.Pipeline, decisionConfidence(decision), truncateRunes(decision.Query, 60))

	// 直连管线：命中后直接返回结果，不进搜索引擎与浏览器
	if decision.Pipeline == PipelineWeather {
		ctx, cancel := context.WithTimeout(context.Background(), weather.QueryTimeout)
		data, err := weather.Query(ctx, weatherCity(decision))
		cancel()
		if err == nil {
			logger.Info("CrystalAstral", "Web-Search 天气直连完成（跳过检索）: %s", data.City)
			return weather.Render(data), nil
		}
		loggerWarn("天气查询失败，回退检索管线: %v", err)
		decision.Query = instruction
	}
	query := decision.Query

	var cache *PageCache
	if cfg.cacheEnabled() {
		if pc, perr := openPageCache(time.Duration(cfg.CacheTTLDays) * 24 * time.Hour); perr == nil {
			cache = pc
			defer pc.close()
		} else {
			loggerWarn("近期访问缓存不可用，本次全部实时访问: %v", perr)
		}
	}

	// 步骤 1：打开「网页搜索:首页」，搜索降级链 + 完整滚动提取，完成后立即关闭首页
	// （首页配额 = 总提取上限 ÷ 计划页面条目数，与详情页均分一致）
	home, err := browser.OpenWebViewSession("", "网页搜索:首页", 1280, 860)
	if err != nil {
		return "", fmt.Errorf("打开浏览器会话失败: %w", err)
	}
	outcome, searchErr := searchWithFallback(home, query, cfg.perPageChars(cfg.detailCount()+1), cfg, deadline)
	home.Close() // 首页使命完成，无论成败都关闭
	if searchErr != nil {
		return "", searchErr
	}
	logger.Info("CrystalAstral", "Web-Search 使用引擎 [%s]: %d 条结果", outcome.Engine.name, len(outcome.Extract.Results))

	// 步骤 2：筛选需打开的详情页链接（指令中的「前N个」优先于配置的条目余量）
	n := cfg.detailCount()
	if m := pageCountFromInstruction(instruction); m > 0 {
		n = m
	}
	if n > webSearchPagesHardCap-1 {
		n = webSearchPagesHardCap - 1
	}
	targets := planDetailPages(outcome, cache, n)
	if len(targets) == 0 {
		return "", fmt.Errorf("结果页中没有可打开的详情页（全部为重复/近期已访问）")
	}

	// 步骤 3：用「网页搜索:详情」同时打开全部详情页，全部加载后统一提取，随后统一关窗
	captures, interrupted := collectDetailPages(targets, cfg, deadline)
	cacheCaptures(cache, captures)

	// 步骤 4：把全部页面正文拼接为长文，一次模型调用完成针对性摘要提取
	// （以月华原始指令为准绳剔除噪音：只保留与所需信息直接相关的内容）
	digest, digestErr := composeDigest(instruction, query, buildDigestSources(outcome.SearchText, captures), profile)
	if digestErr != nil {
		loggerWarn("摘要提取失败，输出将走确定性兜底: %v", digestErr)
	}

	// 步骤 5：摘要提取结果即最终输出（失败时兜底）
	in := reportInput{
		Instruction:    instruction,
		Query:          query,
		EngineName:     engineDisplayName(outcome.Engine.name),
		Engine:         outcome.Extract,
		SearchPageText: outcome.SearchText,
		Digest:         digest,
		Pages:          captures,
		Interrupted:    interrupted,
		Elapsed:        time.Since(started).Truncate(time.Second).String(),
	}
	report := composeReport(in)
	recordLastSearch(query, outcome.Engine.name, report, len(captures), started)

	// 步骤 6：返回前由 defer 卸载搜索模型，随后把最终输出交给月华
	logger.Info("CrystalAstral", "Web-Search 网络搜索完成: 引擎=%s, 检索词=%s, 页面=%d/%d, 中断=%v, 耗时=%s",
		outcome.Engine.name, query, len(captures), n, interrupted, time.Since(started).Truncate(time.Second))
	return report, nil
}

// buildDigestSources 组装统一摘要的正文来源：搜索结果页在前，详情页按打开顺序在后
func buildDigestSources(searchText string, captures []PageCapture) []digestSource {
	sources := make([]digestSource, 0, len(captures)+1)
	if strings.TrimSpace(searchText) != "" {
		sources = append(sources, digestSource{
			Label: "搜索结果页",
			Title: "搜索结果页（首页提取）",
			Text:  searchText,
		})
	}
	for i, cap := range captures {
		sources = append(sources, digestSource{
			Label: fmt.Sprintf("详情页 %d", i+1),
			Title: firstNonEmpty(cap.Title, cap.Domain),
			URL:   cap.URL,
			Text:  cap.Text,
		})
	}
	return sources
}

// pageCountFromInstruction 从指令中解析「前N个/页/条」的数量要求（无则 0）
func pageCountFromInstruction(instruction string) int {
	m := pageCountRe.FindStringSubmatch(instruction)
	if len(m) < 2 {
		return 0
	}
	digits := map[string]string{
		"０": "0", "１": "1", "２": "2", "３": "3", "４": "4",
		"５": "5", "６": "6", "７": "7", "８": "8", "９": "9",
	}
	for fw, ascii := range digits {
		m[1] = strings.ReplaceAll(m[1], fw, ascii)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
