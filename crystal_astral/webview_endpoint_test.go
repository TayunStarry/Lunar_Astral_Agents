package main

// WebView 深读代理端到端测试（真实环境）：
// 打开顶层 WebView 会话访问必应搜索结果页，验证元素识别（DOM 提取）、滚动追加、
// 窗口截图与模型摘要的完整链路。测试会短暂弹出深读代理窗口，结束后自动关闭。

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebviewDeepReadE2E(t *testing.T) {
	body := `{"url":"https://cn.bing.com/search?q=%E5%8D%97%E4%BA%AC%E5%A4%A9%E6%B0%94","query_hint":"南京天气","scroll_rounds":1,"max_results":15,"with_summary":true,"want_screenshot":true}`
	req := httptest.NewRequest("POST", "/webview/deepread", strings.NewReader(body))
	rec := httptest.NewRecorder()

	webviewDeepReadHandler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("HTTP 状态 %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success        bool   `json:"success"`
		SessionID      string `json:"session_id"`
		Kind           string `json:"kind"`
		URL            string `json:"url"`
		Title          string `json:"title"`
		ReadyState     string `json:"ready_state"`
		Results        []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"results"`
		ResultsCount int    `json:"results_count"`
		Summary      string `json:"summary"`
		SummaryError string `json:"summary_error"`
		Screenshot   string `json:"screenshot"`
		ReadyWarning string `json:"ready_warning"`
		Error        string `json:"error"`
		ElapsedMs    int64  `json:"elapsed_ms"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v\n%s", err, rec.Body.String())
	}

	defer func() {
		if resp.SessionID != "" {
			_ = closeWebviewSessionForTest(resp.SessionID)
		}
	}()

	if !resp.Success {
		t.Fatalf("深读失败: %s", resp.Error)
	}
	t.Logf("会话=%s 就绪状态=%s 耗时=%dms url=%s 标题=%s", resp.SessionID, resp.ReadyState, resp.ElapsedMs, resp.URL, resp.Title)
	if resp.ReadyWarning != "" {
		t.Logf("就绪警告: %s", resp.ReadyWarning)
	}

	if resp.Kind != "search" {
		t.Errorf("页面类型应为 search，实际 %s", resp.Kind)
	}
	if len(resp.Results) < 3 {
		t.Errorf("搜索结果应 ≥3 条，实际 %d 条", len(resp.Results))
	}
	for i, hit := range resp.Results {
		if i >= 3 {
			break
		}
		if hit.Title == "" || hit.URL == "" {
			t.Errorf("结果 %d 缺少标题或链接: %+v", i, hit)
		}
		t.Logf("结果[%d] %s | %s | %s", i, hit.Title, hit.URL, truncateRunes(hit.Snippet, 60))
	}

	if resp.Screenshot == "" {
		t.Logf("截图缺失（可能被遮挡）")
	} else {
		t.Logf("截图 dataURL 长度 %d", len(resp.Screenshot))
	}

	if resp.SummaryError != "" {
		t.Logf("模型摘要未生成: %s", resp.SummaryError)
	} else if strings.TrimSpace(resp.Summary) == "" {
		t.Errorf("模型摘要为空")
	} else {
		t.Logf("模型摘要:\n%s", resp.Summary)
	}
}

func closeWebviewSessionForTest(id string) error {
	req := httptest.NewRequest("POST", "/webview/close", strings.NewReader(`{"session_id":"`+id+`"}`))
	rec := httptest.NewRecorder()
	webviewCloseHandler(rec, req)
	return nil
}
