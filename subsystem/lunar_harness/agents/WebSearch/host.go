package WebSearch

// Web-Search · 网络搜索内置智能体（web_search）
//
//	步骤 0：卸载 system-multimodal → 加载 system-search → 搜索模型提炼检索词
//	步骤 1：打开搜索结果页，从顶部到底部完整滚动并提取全部正文
//	步骤 2：筛选指定数量的近期未访问链接，逐个进入详情页滚动并提取正文
//	步骤 3：任一页面提取不到有效正文时直接舍弃该网页
//	步骤 4：滚动过程中同步摘要，触底后停留等待摘要完成
//	步骤 5：所有详情页完成后回到搜索结果主页面，整体汇总、归纳与结构化排版
//	步骤 6：卸载 system-search（主模型交给 router 按需重载）
//	步骤 7：返回最终搜索报告
//
// 全程只使用文本，不依赖任何截图或视觉识别路径；业务完成后关闭浏览器页面。
// 配置来自 engine/config 统一解析的 web_search 节。

import (
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
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

	// 步骤 0：卸载多模态模型，加载搜索模型，并提炼检索词（切换失败时退回主模型继续）
	profile, unloadSearchModel := prepareSearchModel(cfg)
	defer unloadSearchModel()
	query := ExtractQuery(instruction, profile)

	// 步骤 1：打开独立浏览器会话，搜索引擎降级链 + 完整滚动 + 正文提取
	sess, err := browser.OpenWebViewSession("", "琉璃 · 网络搜索", 1280, 860)
	if err != nil {
		return "", fmt.Errorf("打开浏览器会话失败: %w", err)
	}
	defer sess.Close()

	var cache *PageCache
	if cfg.cacheEnabled() {
		if pc, perr := openPageCache(time.Duration(cfg.CacheTTLDays) * 24 * time.Hour); perr == nil {
			cache = pc
			defer pc.close()
		} else {
			loggerWarn("近期访问缓存不可用，本次全部实时访问: %v", perr)
		}
	}

	outcome, err := searchWithFallback(sess, query, cfg, deadline)
	if err != nil {
		return "", err
	}
	logger.Info("CrystalAstral", "Web-Search 使用引擎 [%s]: %d 条结果", outcome.Engine.name, len(outcome.Extract.Results))

	// 步骤 2-4：筛选指定数量的近期未访问链接，逐页滚动并同步摘要
	n := cfg.MaxPages
	if m := pageCountFromInstruction(instruction); m > 0 {
		n = m
	}
	if n > webSearchPagesHardCap {
		n = webSearchPagesHardCap
	}
	intels, interrupted := collectDetailPages(sess, outcome, query, cfg, cache, profile, n, deadline)

	// 步骤 5：回到搜索结果主页面，整体汇总、归纳与结构化排版
	searchText := revisitSearchPage(sess, outcome, cfg, deadline)
	in := reportInput{
		Instruction:    instruction,
		Query:          query,
		EngineName:     engineDisplayName(outcome.Engine.name),
		Engine:         outcome.Extract,
		SearchPageText: searchText,
		Pages:          intels,
		Interrupted:    interrupted,
		Elapsed:        time.Since(started).Truncate(time.Second).String(),
	}
	report := composeReport(in, profile)

	// 步骤 6-7：返回前由 defer 卸载搜索模型，随后把报告交给月华
	logger.Info("CrystalAstral", "Web-Search 网络搜索完成: 引擎=%s, 检索词=%s, 页面=%d/%d, 中断=%v, 耗时=%s",
		outcome.Engine.name, query, len(intels), n, interrupted, time.Since(started).Truncate(time.Second))
	return report, nil
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
