package handlers

import (
	"browser"
	"config"
	"encoding/json"
	"fmt"
	"net/http"
)

// WebViewReopenResponse webView重建响应结构体
type WebViewReopenResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// WebViewReopenHandler 重建webView页面端点
// 仅当webView已关闭时才重新创建，若webView仍在运行则跳过
func WebViewReopenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if browser.IsWebViewRunning() {
		// webView仍在运行，无需重建
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WebViewReopenResponse{
			Success: true,
			Message: "webView仍在运行，无需重建",
		})
		return
	}

	// webView已关闭，重新创建
	clientURL := fmt.Sprintf("http://127.0.0.1:%d", *config.BasicPort)
	browser.OpenBrowser(clientURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(WebViewReopenResponse{
		Success: true,
		Message: "webView重建请求已发送",
	})
}
