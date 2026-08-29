package handlers

import (
	"LunarAstral/adapters"
	"encoding/json"
	"net/http"
)

// LTPXRemoteRegisterHandler 处理琉璃启动时提交联络 URL 的请求（POST /ltpx/register）
// 月华固定端口，琉璃随机端口；多个琉璃进程以最新注册为准，只记录一个 URL
func LTPXRemoteRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req adapters.LTPXRemoteRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeLTPXJSON(w, http.StatusBadRequest, LTPXRemoteRegisterResponse{
			Success: false, Message: "无效的请求体: " + err.Error(),
		})
		return
	}

	if req.URL == "" {
		writeLTPXJSON(w, http.StatusBadRequest, LTPXRemoteRegisterResponse{
			Success: false, Message: "url 为必填项",
		})
		return
	}

	// 记录琉璃联络 URL（以最新为准）
	adapters.RegisterLTPXRemoteURL(req.URL)

	writeLTPXJSON(w, http.StatusOK, LTPXRemoteRegisterResponse{
		Success: true, Message: "琉璃联络 URL 已注册: " + req.URL,
	})
}

func writeLTPXJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}