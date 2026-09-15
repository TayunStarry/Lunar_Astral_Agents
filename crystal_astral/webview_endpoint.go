package main

// ==== WebView 深读代理（后端驱动的顶层 WebView 会话） ====
//
// 为 AtoA 包提供「真浏览器」能力，突破内嵌 iframe 的三项跨源限制：
//   - 顶层窗口不受 X-Frame-Options 约束，任意结果页可打开
//   - Eval 在页面上下文执行脚本 → 元素识别（DOM 提取，无 OCR 误差）
//   - 可滚动翻页，覆盖首屏之外的内容
//
// 端点族（会话由 BrowserClient.WebViewSession 管理，多实例、独立 OS 线程）：
//   POST /webview/open       创建会话窗口
//   POST /webview/navigate   会话导航
//   POST /webview/eval       页面上下文执行脚本并取回 JSON 结果
//   POST /webview/ready      等待页面就绪
//   POST /webview/screenshot 会话窗口区域截图（屏幕 DC，dataURL 返回）
//   POST /webview/close      关闭会话
//   POST /webview/deepread   高层编排：导航 → 就绪 → 元素提取（可滚动） → 截图 → 模型摘要

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	imageproc "LunarSubsystem/MultimodalAnalysis/module"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// deepreadMu 全局深读编排互斥：同一时刻只跑一次深读（会话内的 Eval 不可并发）
var deepreadMu sync.Mutex

// webviewOpenRequest POST /webview/open 请求体
type webviewOpenRequest struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// webviewOpenHandler 创建会话窗口
func webviewOpenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewOpenRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	sess, err := BrowserClient.OpenWebViewSession(req.URL, req.Title, req.Width, req.Height)
	if err != nil {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonOK(w, http.StatusOK, map[string]any{"success": true, "session_id": sess.ID, "title": sess.Title})
}

// webviewNavigateRequest POST /webview/navigate 请求体
type webviewNavigateRequest struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
	TimeoutMs int    `json:"timeout_ms"`
}

// webviewNavigateHandler 会话导航并等待就绪
func webviewNavigateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewNavigateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "无效的请求体（需 url）", http.StatusBadRequest)
		return
	}
	sess, ok := resolveWebviewSession(req.SessionID)
	if !ok {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": "无可用会话（先调用 /webview/open）"})
		return
	}
	prev := ""
	if st, err := sess.Snapshot(); err == nil {
		prev = st.URL
	}
	if err := sess.Navigate(req.URL); err != nil {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	st, err := sess.WaitReady(prev, timeout)
	resp := map[string]any{"success": err == nil, "session_id": sess.ID, "ready_state": st.ReadyState, "url": st.URL, "title": st.Title}
	if err != nil {
		resp["error"] = err.Error()
	}
	jsonOK(w, http.StatusOK, resp)
}

// webviewEvalRequest POST /webview/eval 请求体
type webviewEvalRequest struct {
	SessionID string `json:"session_id"`
	Script    string `json:"script"`
	TimeoutMs int    `json:"timeout_ms"`
}

// webviewEvalHandler 页面上下文执行脚本（script 内 return 值经 JSON 序列化回传）
func webviewEvalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewEvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Script) == "" {
		http.Error(w, "无效的请求体（需 script）", http.StatusBadRequest)
		return
	}
	sess, ok := resolveWebviewSession(req.SessionID)
	if !ok {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": "无可用会话（先调用 /webview/open）"})
		return
	}
	value, err := sess.EvalJSON(req.Script, time.Duration(req.TimeoutMs)*time.Millisecond)
	resp := map[string]any{"success": err == nil, "session_id": sess.ID, "value": value}
	if err != nil {
		resp["error"] = err.Error()
	}
	jsonOK(w, http.StatusOK, resp)
}

// webviewReadyHandler 等待页面就绪并返回状态
func webviewReadyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewNavigateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = webviewNavigateRequest{}
	}
	sess, ok := resolveWebviewSession(req.SessionID)
	if !ok {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": "无可用会话（先调用 /webview/open）"})
		return
	}
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	st, err := sess.WaitReady("", timeout)
	resp := map[string]any{"success": err == nil, "session_id": sess.ID, "ready_state": st.ReadyState, "url": st.URL, "title": st.Title}
	if err != nil {
		resp["error"] = err.Error()
	}
	jsonOK(w, http.StatusOK, resp)
}

// webviewScreenshotRequest POST /webview/screenshot 请求体
type webviewScreenshotRequest struct {
	SessionID string `json:"session_id"`
	Quality   int    `json:"quality"`
}

// webviewScreenshotHandler 会话窗口区域截图（屏幕 DC，可正确截到 GPU 合成内容）
func webviewScreenshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewScreenshotRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	sess, ok := resolveWebviewSession(req.SessionID)
	if !ok {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": "无可用会话（先调用 /webview/open）"})
		return
	}
	dataURL, err := webviewSessionScreenshot(sess, req.Quality)
	resp := map[string]any{"success": err == nil, "session_id": sess.ID}
	if err != nil {
		resp["error"] = err.Error()
	} else {
		resp["screenshot"] = dataURL
	}
	jsonOK(w, http.StatusOK, resp)
}

// webviewCloseHandler 关闭会话
func webviewCloseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewNavigateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	var err error
	if req.SessionID == "" {
		if sess, ok := BrowserClient.LatestWebViewSession(); ok {
			err = BrowserClient.CloseWebViewSession(sess.ID)
		} else {
			err = fmt.Errorf("无可用会话")
		}
	} else {
		err = BrowserClient.CloseWebViewSession(req.SessionID)
	}
	jsonOK(w, http.StatusOK, map[string]any{"success": err == nil, "error": errString(err)})
}

// ==== 深读编排 ====

// webviewDeepReadRequest POST /webview/deepread 请求体
type webviewDeepReadRequest struct {
	URL            string `json:"url"`
	QueryHint      string `json:"query_hint"`      // 检索意图提示（供摘要聚焦）
	ScrollRounds   int    `json:"scroll_rounds"`   // 额外向下滚动的轮数（0-5）
	MaxResults     int    `json:"max_results"`     // 搜索结果条数上限（默认 15）
	WithSummary    bool   `json:"with_summary"`    // 是否调用模型生成摘要（默认 true）
	WantScreenshot bool   `json:"want_screenshot"` // 是否返回窗口截图 dataURL
	SessionID      string `json:"session_id"`
}

// webSearchHit 搜索结果/链接条目
type webSearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// webExtract 页面元素提取结果
type webExtract struct {
	Kind    string         `json:"kind"` // search=搜索结果页 page=普通网页
	URL     string         `json:"url"`
	Title   string         `json:"title"`
	Results []webSearchHit `json:"results"`
	Links   []webSearchHit `json:"links"`
	Text    string         `json:"text"`
}

// webviewDeepReadHandler 深读编排：导航 → 就绪 → 元素提取（可滚动） → 截图 → 模型摘要
func webviewDeepReadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req webviewDeepReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, "无效的请求体（需 url）", http.StatusBadRequest)
		return
	}
	if req.ScrollRounds < 0 {
		req.ScrollRounds = 0
	}
	if req.ScrollRounds > 5 {
		req.ScrollRounds = 5
	}
	if req.MaxResults <= 0 {
		req.MaxResults = 15
	}

	deepreadMu.Lock()
	defer deepreadMu.Unlock()

	started := time.Now()
	sess, err := ensureWebviewSession(req.SessionID)
	if err != nil {
		jsonOK(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}

	resp := map[string]any{"success": true, "session_id": sess.ID}

	prev := ""
	if st, err := sess.Snapshot(); err == nil {
		prev = st.URL
	}
	if err := sess.Navigate(req.URL); err != nil {
		resp["success"], resp["error"] = false, err.Error()
		jsonOK(w, http.StatusOK, resp)
		return
	}
	st, readyErr := sess.WaitReady(prev, 25*time.Second)
	resp["url"], resp["title"], resp["ready_state"] = st.URL, st.Title, st.ReadyState
	resp["ready_ms"] = time.Since(started).Milliseconds()
	if readyErr != nil {
		resp["ready_warning"] = readyErr.Error()
	}
	time.Sleep(1200 * time.Millisecond) // 渲染稳定

	// 元素提取 + 可选滚动追加
	page, extErr := webviewExtractPage(sess, req.MaxResults)
	for i := 0; i < req.ScrollRounds; i++ {
		if _, err := sess.EvalJSON(`return window.scrollBy(0, Math.round(window.innerHeight * 0.92)); JSON.stringify({scrollY: window.scrollY});`, 4*time.Second); err != nil {
			break
		}
		time.Sleep(900 * time.Millisecond)
		page2, err := webviewExtractPage(sess, req.MaxResults)
		if err != nil {
			break
		}
		mergeWebExtract(&page, &page2)
	}
	if extErr != nil {
		resp["success"], resp["error"] = false, "元素提取失败: "+errString(extErr)
		jsonOK(w, http.StatusOK, resp)
		return
	}
	resp["kind"] = page.Kind
	resp["url"] = page.URL
	resp["title"] = page.Title
	resp["results"] = page.Results
	resp["links"] = page.Links
	resp["results_count"] = len(page.Results)

	// 窗口截图（可选）
	if req.WantScreenshot {
		if dataURL, err := webviewSessionScreenshot(sess, 80); err == nil {
			resp["screenshot"] = dataURL
		} else {
			resp["screenshot_warning"] = err.Error()
		}
	}

	// 模型摘要（可选，失败不影响提取结果返回）
	if req.WithSummary {
		summary, sumErr := webviewSummarize(page, req.QueryHint)
		if sumErr != nil {
			resp["summary_error"] = sumErr.Error()
		} else {
			resp["summary"] = summary
		}
	}
	resp["elapsed_ms"] = time.Since(started).Milliseconds()
	LoggerGeneral.Info("CrystalAstral", "深读代理完成 url=%s kind=%s results=%d summary=%v elapsed=%s",
		req.URL, page.Kind, len(page.Results), resp["summary"] != nil, time.Since(started).Truncate(time.Millisecond))
	jsonOK(w, http.StatusOK, resp)
}

// ensureWebviewSession 解析/创建深读会话（显式 session_id → 最近会话 → 新开窗口）
func ensureWebviewSession(sessionID string) (*BrowserClient.WebViewSession, error) {
	if sessionID != "" {
		if sess, ok := BrowserClient.GetWebViewSession(sessionID); ok && sess.IsAlive() {
			return sess, nil
		}
		return nil, fmt.Errorf("会话不存在或已关闭: %s", sessionID)
	}
	if sess, ok := BrowserClient.LatestWebViewSession(); ok {
		return sess, nil
	}
	sess, err := BrowserClient.OpenWebViewSession("", "琉璃 · 深读代理", 1280, 860)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// resolveWebviewSession 供简单端点解析会话
func resolveWebviewSession(sessionID string) (*BrowserClient.WebViewSession, bool) {
	if sessionID != "" {
		return BrowserClient.GetWebViewSession(sessionID)
	}
	return BrowserClient.LatestWebViewSession()
}

// webviewExtractPage 在页面上下文执行元素提取脚本（必应结果页与普通网页自适应）
func webviewExtractPage(sess *BrowserClient.WebViewSession, maxResults int) (webExtract, error) {
	page := webExtract{}
	raw, err := sess.EvalJSON(webviewExtractScript(maxResults), 8*time.Second)
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return page, fmt.Errorf("解析提取结果失败: %w", err)
	}
	if page.Kind == "" {
		page.Kind = "page"
	}
	if page.Results == nil {
		page.Results = []webSearchHit{}
	}
	if page.Links == nil {
		page.Links = []webSearchHit{}
	}
	return page, nil
}

// mergeWebExtract 合并滚动后的提取结果（按 URL 去重，文本取并集前段）
func mergeWebExtract(base, add *webExtract) {
	seen := map[string]bool{}
	for _, h := range base.Results {
		seen[h.URL] = true
	}
	for _, h := range add.Results {
		if !seen[h.URL] {
			seen[h.URL] = true
			base.Results = append(base.Results, h)
		}
	}
	seenLinks := map[string]bool{}
	for _, h := range base.Links {
		seenLinks[h.URL] = true
	}
	for _, h := range add.Links {
		if !seenLinks[h.URL] {
			seenLinks[h.URL] = true
			base.Links = append(base.Links, h)
		}
	}
	if len(base.Text) < 3000 {
		base.Text = truncateRunes(base.Text+"\n"+add.Text, 6000)
	}
}

// webviewExtractScript 生成页面元素提取脚本（在页面上下文执行，return 对象）
func webviewExtractScript(maxResults int) string {
	return fmt.Sprintf(`return (function(){
    var out = { kind: 'page', url: location.href, title: document.title, results: [], links: [], text: '' };
    var lis = document.querySelectorAll('#b_results > li.b_algo');
    if (lis.length > 0) { out.kind = 'search'; }
    var seen = {};
    lis.forEach(function (li) {
        if (out.results.length >= %d) return;
        var a = li.querySelector('h2 a');
        if (!a || !a.href) return;
        var cap = li.querySelector('.b_caption p') || li.querySelector('.b_caption') || li.querySelector('p');
        out.results.push({
            title: (a.textContent || '').trim(),
            url: a.href,
            snippet: cap ? (cap.textContent || '').trim().slice(0, 320) : ''
        });
        seen[a.href] = 1;
    });
    if (out.kind === 'page') {
        out.text = (document.body ? (document.body.innerText || '') : '').replace(/[\t ]+/g, ' ').replace(/\n{3,}/g, '\n\n').slice(0, 6000);
        var anchors = document.querySelectorAll('a[href]');
        for (var i = 0; i < anchors.length && out.links.length < 25; i++) {
            var a = anchors[i];
            var h = a.href;
            if (!/^https?:/.test(h) || seen[h]) continue;
            var t = (a.textContent || '').trim().replace(/\s+/g, ' ');
            if (t.length > 8 && t.length < 120) { seen[h] = 1; out.links.push({ title: t, url: h }); }
        }
    }
    return out;
})()`, maxResults)
}

// webviewSessionScreenshot 会话窗口区域截图（屏幕 DC）并编码为 JPEG dataURL
func webviewSessionScreenshot(sess *BrowserClient.WebViewSession, quality int) (string, error) {
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	x, y, w, h, err := sess.WindowRect()
	if err != nil {
		return "", err
	}
	img, err := imageproc.CaptureScreenRegionRGBA(x, y, w, h)
	if err != nil {
		return "", fmt.Errorf("区域截图失败: %w", err)
	}
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return "", fmt.Errorf("截图编码失败: %w", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// ==== 模型摘要（元素识别结果 → 中文要点） ====

// webChatMessage /v1 协议对话消息（深读摘要只需文本）
type webChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// webviewSummarize 调用月华多模态模型（/v1 协议）把元素提取结果总结为中文要点
func webviewSummarize(page webExtract, queryHint string) (string, error) {
	model := strings.TrimSpace(*GeneralConfig.AgentMultimodalModel)
	if model == "" {
		model = "system-multimodal"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(*GeneralConfig.AgentMultimodalURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:36789/v1"
	}
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}

	// 构建紧凑载荷：搜索页保留结果条目；普通页保留正文与链接
	payload := map[string]any{
		"query_hint": queryHint,
		"page": map[string]any{
			"title":   page.Title,
			"url":     page.URL,
			"kind":    page.Kind,
			"results": page.Results,
		},
	}
	if page.Kind == "page" {
		payload["page"].(map[string]any)["links"] = page.Links
		payload["page"].(map[string]any)["text"] = truncateRunes(page.Text, 4500)
	}
	payloadJSON, _ := json.Marshal(payload)

	system := "你是网页内容识别与摘要助手。用户给出的页面数据来自浏览器元素识别（DOM 提取，非 OCR，内容准确可信）。" +
		"请用简洁中文输出要点式总结（4-8 条，每条一行，可带短标题），保留关键数字、名称、结论与来源域名；" +
		"若提供了 query_hint，请围绕该检索意图筛选与组织内容；数据不足时如实说明。只输出总结正文，不要输出代码块。"

	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": []webChatMessage{{Role: "system", Content: system}, {Role: "user", Content: string(payloadJSON)}},
		"stream":   false,
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建摘要请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(*GeneralConfig.AgentMultimodalKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("摘要请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取摘要响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("模型 API 返回状态 %d: %s", resp.StatusCode, truncateRunes(string(respBody), 200))
	}
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析摘要响应失败: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("模型 API 错误: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("模型 API 返回空响应")
	}
	summary := webChatText(chatResp.Choices[0].Message.Content)
	if strings.TrimSpace(summary) == "" {
		return "", fmt.Errorf("模型返回空摘要")
	}
	return summary, nil
}

// webChatText 从消息 content（字符串或片段数组）提取纯文本
func webChatText(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// truncateRunes 以 rune 为单位截断
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// errString 错误转字符串（nil → 空串）
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
