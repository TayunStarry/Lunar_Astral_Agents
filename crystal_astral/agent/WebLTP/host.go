package WebLTP

// Web-LTP · 网络搜索主流程（独立 LTPX 内置工具，月华自然语言指令 → 搜索报告）

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/LoggerGeneral"
	"encoding/base64"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runMu 串行化网络搜索运行（共享浏览器窗口与截图资源）
var runMu sync.Mutex

// PageIntel 单个结果页采集到的情报
type PageIntel struct {
	Title      string
	URL        string
	Domain     string
	Summary    string // 已按配置硬切断（≤4096 字符）
	Captures   int
	Failed     string // 打开/提取失败原因（空表示成功）
	SummaryErr string // 模型摘要失败原因（此时摘要为文本前段兜底）
}

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
		dir:     filepath.Join(LocalDirForScreenshots(), cfg.ScreenshotDir, runID),
	}

	searchURL := "https://cn.bing.com/search?q=" + url.QueryEscape(query)
	interrupted := false

	// 3. 打开搜索结果页
	if err := navTo(sess, searchURL, 20*time.Second, 1500*time.Millisecond); err != nil {
		return "", fmt.Errorf("打开搜索结果页失败: %w", err)
	}

	// 4. 结果页滚动截图 → 回到页顶（首屏截图供报告汇编时视觉印证）
	resultsCaptures, engineShot := scrollCapture(sess, cfg.ResultsScrollCaptures, "搜索结果页", saver, deadline)
	_ = scrollToTop(sess)

	// 5. 元素识别结果页（DOM 为主，截图为辅）
	engine, extErr := extractPage(sess, cfg.MaxResults)
	if extErr != nil {
		return "", fmt.Errorf("结果页元素识别失败: %w", extErr)
	}
	LoggerGeneral.Info("CrystalAstral", "Web-LTP 结果页识别完成: %d 条结果, 截图 %d 张", len(engine.Results), resultsCaptures)

	// 6. 依次进入前 N 个结果页（指令可覆盖数量，硬上限 10）
	n := cfg.MaxPages
	if m := pageCountFromInstruction(instruction); m > 0 {
		n = m
	}
	if n > webSearchPagesHardCap {
		n = webSearchPagesHardCap
	}
	if n > len(engine.Results) {
		n = len(engine.Results)
	}

	intels := []PageIntel{}
	for i := 0; i < n; i++ {
		if !sess.IsAlive() {
			interrupted = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		hit := engine.Results[i]
		intel := PageIntel{Title: hit.Title, URL: hit.URL, Domain: domainOf(hit.URL)}

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

		summary, sumErr := summarizePage(query, hit, page, pageShot)
		if sumErr != nil {
			// 模型摘要失败：以正文前段兜底（同样硬切断）
			intel.Summary = hardCutRunes(firstNonEmpty(page.Text, "（页面无可提取文本）"), cfg.SummaryMaxChars)
			intel.SummaryErr = sumErr.Error()
		} else {
			intel.Summary = hardCutRunes(summary, cfg.SummaryMaxChars)
		}
		intels = append(intels, intel)
		LoggerGeneral.Info("CrystalAstral", "Web-LTP 页面 %d/%d 完成: %s (摘要 %d 字, 截图 %d 张)",
			i+1, n, intel.Domain, len([]rune(intel.Summary)), intel.Captures)
	}

	// 7. 回到搜索引擎页（窗口仍存活时），并补一张引擎页截图
	if sess.IsAlive() && !time.Now().After(deadline) {
		if err := navTo(sess, searchURL, 15*time.Second, 1200*time.Millisecond); err == nil {
			if jpg, err := shotCapture(sess, 85); err == nil {
				saver.save(jpg, "搜索引擎页-回访")
				if engineShot == "" {
					engineShot = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg)
				}
			}
		}
	}

	// 8. 汇编搜索报告（月华操作旅程口吻；附结果页首屏截图供视觉印证）
	report := composeReport(reportInput{
		Instruction: instruction,
		Query:       query,
		Engine:      engine,
		EngineShot:  engineShot,
		Pages:       intels,
		Screenshots: saver.count,
		SaveDir:     captureDirForReport(saver),
		Interrupted: interrupted,
		Elapsed:     time.Since(started).Truncate(time.Second).String(),
	})
	LoggerGeneral.Info("CrystalAstral", "Web-LTP 网络搜索完成: 检索词=%s, 页面=%d/%d, 截图=%d, 中断=%v, 耗时=%s",
		query, len(intels), n, saver.count, interrupted, time.Since(started).Truncate(time.Second))
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

// LocalDirForScreenshots 本地数据根目录（截图保存基于 LocalDir）
func LocalDirForScreenshots() string {
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
