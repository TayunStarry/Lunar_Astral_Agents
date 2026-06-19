package handlers

import (
	"encoding/json"
	"lunar_astral/adapters"
	"net/http"
)

// LTPXLoadHandler 处理工具加载请求
func LTPXLoadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LTPXLoadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeLTPXResponse(w, http.StatusBadRequest, LTPXResponse{
			Success: false, Message: "无效的请求体: " + err.Error(),
		})
		return
	}

	if req.Name == "" || req.Definition == "" || req.JS == "" {
		writeLTPXResponse(w, http.StatusBadRequest, LTPXResponse{
			Success: false, Message: "name、tool_definition 和 tool_js 均为必填项",
		})
		return
	}

	// 在 goja 事件循环中执行加载
	adapters.LoadLTPXToolOnLoop(req.Name, req.Definition, req.JS)

	writeLTPXResponse(w, http.StatusOK, LTPXResponse{
		Success: true, Message: "工具 " + req.Name + " 加载成功",
	})
}

// LTPXUnloadHandler 处理工具卸载请求
func LTPXUnloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LTPXUnloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeLTPXResponse(w, http.StatusBadRequest, LTPXResponse{
			Success: false, Message: "无效的请求体: " + err.Error(),
		})
		return
	}

	if req.Name == "" {
		writeLTPXResponse(w, http.StatusBadRequest, LTPXResponse{
			Success: false, Message: "name 为必填项",
		})
		return
	}

	// 在 goja 事件循环中执行卸载
	adapters.UnloadLTPXToolOnLoop(req.Name)

	writeLTPXResponse(w, http.StatusOK, LTPXResponse{
		Success: true, Message: "工具 " + req.Name + " 卸载成功",
	})
}

// LTPXStatusHandler 查询工具状态
func LTPXStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := adapters.GetLTPXToolStatus()
	writeLTPXResponse(w, http.StatusOK, LTPXResponse{
		Success: true, Data: status,
	})
}

func writeLTPXResponse(w http.ResponseWriter, statusCode int, resp LTPXResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}
