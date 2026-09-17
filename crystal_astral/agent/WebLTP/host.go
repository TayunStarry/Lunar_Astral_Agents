package WebLTP

// Web-LTP · 网络搜索主流程（独立 LTPX 内置工具，月华自然语言指令 → 搜索报告）

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/LoggerGeneral"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runMu 串行化网络搜索运行（共享浏览器窗口与截图资源）
var runMu sync.Mutex


// Run 执行一次网络搜索：月华自然语言指令 → 搜索报告（文本）。
// 全流程由程序自主依次完成，不依赖前端包；业务完成后关闭浏览器页面；
// 浏览器窗口意外关闭时立即停止后续操作，以已采集信息构建报告。
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
	runID := started.Format("20060102-150405")
	LoggerGeneral.Info("CrystalAstral", "Web-LTP 网络搜索开始: %s", truncateRunes(instruction, 80))

	// 1. 提炼检索词（失败回退原指令）
	query := ExtractQuery(instruction)

	// 2. 打开独立浏览器会话（业务结束后统一关闭）
	sess, err := BrowserClient.OpenWebViewSession("", "琉璃 · 网络搜索", 1280, 860)
	if err != nil {
		return "", fmt.Errorf("打开浏览器会话失败: %w", err)
	}
	defer sess.Close() // 无论成败，结束时关闭浏览器页面

	saver := &shotSaver{
		enabled: cfg.SaveScreenshots,
		dir:     filepath.Join(LocalDirForData(), cfg.ScreenshotDir, runID),
	}

	// 3. 打开页面摘要缓存（SQL；不可用时为 nil，get/put 均兼容空值，全部实时访问）
	var cache *PageCache
	if cfg.cacheEnabled() {
		if pc, perr := openPageCache(pageCachePath(), time.Duration(cfg.CacheTTLDays)*24*time.Hour); perr == nil {
			cache = pc
			defer pc.close()
		} else {
			LoggerGeneralWarn("页面摘要缓存不可用，本次全部实时访问: %v", perr)
		}
	}

	// 4. 搜索引擎降级链（bing → baidu → sogou）：首个有效结果页胜出
	interrupted := false // 浏览器窗口是否在中途被关闭
	outcome, err := searchWithFallback(sess, query, cfg, saver, deadline)
	if err != nil {
		return "", err
	}
	engine := outcome.Extract
	engineShot := outcome.FirstShot
	LoggerGeneral.Info("CrystalAstral", "Web-LTP 使用引擎 [%s]: %d 条结果", outcome.Engine.name, len(engine.Results))

	// 5. 依次处理前 N 个结果页（指令可覆盖数量，硬上限 10）
	n := cfg.MaxPages
	if m := pageCountFromInstruction(instruction); m > 0 {
		n = m
	}
	if n > webSearchPagesHardCap {
		n = webSearchPagesHardCap
	}

	intels := []PageIntel{}
	visited := map[string]bool{} // 跨结果页 URL 去重：同一页面只进入一次流水线
	for i := 0; i < len(engine.Results) && len(intels) < n; i++ {
		if !sess.IsAlive() {
			interrupted = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		hit := engine.Results[i]
		key := normalizeURL(hit.URL)
		if key == "" || visited[key] {
			LoggerGeneral.Info("CrystalAstral", "Web-LTP 跳过重复/无效链接: %s", truncateRunes(hit.URL, 80))
			continue
		}
		visited[key] = true
		intel := PageIntel{Title: hit.Title, URL: hit.URL, Domain: domainOf(hit.URL)}

		// 缓存命中（未过期）→ 复用已有摘要：不访问网页、不截图、不调用模型
		if entry, ok := cache.get(key); ok {
			intel.Title = firstNonEmpty(entry.Title, intel.Title)
			intel.Domain = firstNonEmpty(entry.Domain, intel.Domain)
			intel.Summary = entry.Summary
			intel.Cached = true
			intels = append(intels, intel)
			LoggerGeneral.Info("CrystalAstral", "Web-LTP 页面 %d 命中缓存，复用摘要: %s (摘要 %d 字)",
				len(intels), intel.Domain, len([]rune(intel.Summary)))
			continue
		}

		if err := navTo(sess, hit.URL, 15*time.Second, 1200*time.Millisecond); err != nil {
			intel.Failed = err.Error()
			intels = append(intels, intel)
			continue
		}

		// 页面滚动截图（≤ PageScrollCaptures，触底即止）；首屏截图供摘要时视觉印证
		label := fmt.Sprintf("页面%02d-%s", i+1, intel.Domain)
		var pageShot string
		intel.Captures, pageShot = scrollCapture(sess, cfg.PageScrollCaptures, label, saver, deadline)

		// 页面内容元素提取（DOM 为主）+ 情报摘要（文本 + 首屏截图互相印证）
		page, err := extractPage(sess, 0)
		if err != nil {
			intel.Failed = "内容提取失败: " + err.Error()
			intels = append(intels, intel)
			continue
		}
		intel.Title = firstNonEmpty(page.Title, intel.Title)
		// 以导航后的最终 URL 为准（百度/搜狗结果为跳转链接，此处还原真实地址）
		if u := strings.TrimSpace(page.URL); u != "" {
			intel.URL = u
			intel.Domain = domainOf(u)
		}

		summary, sumErr := summarizePage(query, hit, page, pageShot)
		if sumErr != nil {
			// 模型摘要失败：以正文前段兜底（同样硬切断）
			intel.Summary = hardCutRunes(firstNonEmpty(page.Text, "（页面无可提取文本）"), cfg.SummaryMaxChars)
			intel.SummaryErr = sumErr.Error()
		} else {
			intel.Summary = hardCutRunes(summary, cfg.SummaryMaxChars)
		}
		intels = append(intels, intel)

		// 摘要写缓存：以最终 URL 为键覆写；跳转型结果链接的地址不稳定，不作缓存键
		if cache != nil && intel.Failed == "" {
			cache.put(normalizeURL(intel.URL), intel.Title, intel.Domain, intel.Summary, query)
			if k2 := normalizeURL(hit.URL); k2 != normalizeURL(intel.URL) && k2 != "" && !isRedirectWrapper(k2) {
				cache.put(k2, intel.Title, intel.Domain, intel.Summary, query)
			}
		}
		LoggerGeneral.Info("CrystalAstral", "Web-LTP 页面 %d/%d 完成: %s (摘要 %d 字, 截图 %d 张)",
			len(intels), n, intel.Domain, len([]rune(intel.Summary)), intel.Captures)
	}

	// 6. 回到搜索引擎页（窗口仍存活时），并补一张引擎页截图
	if sess.IsAlive() && !time.Now().After(deadline) {
		if err := navTo(sess, outcome.URL, 15*time.Second, 1200*time.Millisecond); err == nil {
			if jpg, err := shotCapture(sess, 85); err == nil {
				saver.save(jpg, "搜索引擎页-回访")
				if engineShot == "" {
					engineShot = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg)
				}
			}
		}
	}

	// 7. 汇编搜索报告（月华操作旅程口吻；附结果页首屏截图供视觉印证）
	report := composeReport(reportInput{
		Instruction: instruction,
		Query:       query,
		EngineName:  engineDisplayName(outcome.Engine.name),
		Engine:      engine,
		EngineShot:  engineShot,
		Pages:       intels,
		Screenshots: saver.count,
		SaveDir:     captureDirForReport(saver),
		Interrupted: interrupted,
		Elapsed:     time.Since(started).Truncate(time.Second).String(),
	})
	LoggerGeneral.Info("CrystalAstral", "Web-LTP 网络搜索完成: 引擎=%s, 检索词=%s, 页面=%d/%d, 截图=%d, 中断=%v, 耗时=%s",
		outcome.Engine.name, query, len(intels), n, saver.count, interrupted, time.Since(started).Truncate(time.Second))
	return report, nil
}

// navTo 导航并等待就绪 + 渲染稳定。仅导航调用本身失败才返回错误；
// 就绪等待超时仅记日志（后续元素提取自行决定成败）。
func navTo(sess *BrowserClient.WebViewSession, target string, readyTimeout, settle time.Duration) error {
	prev := ""
	if st, err := sess.Snapshot(); err == nil {
		prev = st.URL
	}
	if err := sess.Navigate(target); err != nil {
		return err
	}
	if _, err := sess.WaitReady(prev, readyTimeout); err != nil {
		LoggerGeneral.SubWarn("CrystalAstral", "WebLTP", "页面就绪等待超时（继续尝试提取）: %v", err)
	}
	time.Sleep(settle)
	return nil
}

// scrollCapture 在当前页面执行「截图 → 下滚一屏」循环（触底或达到上限即止）。
// 返回截图张数与首屏截图的 dataURL（供多模态模型与 DOM 文本互相印证）。
func scrollCapture(sess *BrowserClient.WebViewSession, max int, label string, saver *shotSaver, deadline time.Time) (int, string) {
	count := 0
	firstDataURL := ""
	for i := 0; i < max; i++ {
		if !sess.IsAlive() || time.Now().After(deadline) {
			break
		}
		if jpg, err := shotCapture(sess, 85); err == nil {
			saver.save(jpg, fmt.Sprintf("%s-视口%02d", label, i+1))
			count++
			if firstDataURL == "" {
				firstDataURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg)
			}
		}
		st, err := scrollByViewport(sess)
		if err != nil || st.AtBottom || st.After <= st.Before {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return count, firstDataURL
}

// pageCountFromInstruction 从指令中解析「前N个/页/条」的数量要求（无则 0）
var pageCountRe = regexp.MustCompile(`前\s*([0-9]+|[０-９]+)\s*[个页条]`)

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

// captureDirForReport 报告中的截图目录说明
func captureDirForReport(saver *shotSaver) string {
	if !saver.enabled {
		return ""
	}
	return saver.dir
}

// LocalDirForData 本地数据根目录（截图保存基于 LocalDir；页面摘要缓存已统一到 knowledge.db）
func LocalDirForData() string {
	p := *localDirFlag()
	if p == "" {
		p = "local_data"
	}
	return p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
