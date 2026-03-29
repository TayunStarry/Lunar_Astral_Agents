package handlers

import (
	"Lunar-Astral-Agents/utils"
	"encoding/json"
	"net/http"
)

// WebViewControlRequest WebView 控制请求结构
type WebViewControlRequest struct {
	Action string `json:"action"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	X      int    `json:"x,omitempty"`
	Y      int    `json:"y,omitempty"`
}

// WebViewControlResponse WebView 控制响应结构
type WebViewControlResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// WebViewControlHandler 处理 WebView 控制请求
func WebViewControlHandler(w http.ResponseWriter, r *http.Request) {
	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 解析请求体
	var req WebViewControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(WebViewControlResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	// 处理不同的控制动作
	switch req.Action {
	case "set_size":
		if req.Width > 0 && req.Height > 0 {
			utils.SetWebViewSize(req.Width, req.Height)
			json.NewEncoder(w).Encode(WebViewControlResponse{
				Success: true,
				Message: "WebView size updated",
			})
		} else {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(WebViewControlResponse{
				Success: false,
				Message: "Invalid width or height",
			})
		}

	case "set_position":
		utils.SetWebViewPosition(req.X, req.Y)
		json.NewEncoder(w).Encode(WebViewControlResponse{
			Success: true,
			Message: "WebView position updated (if supported)",
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(WebViewControlResponse{
			Success: false,
			Message: "Invalid action",
		})
	}
}
