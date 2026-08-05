package handlers

import (
	"encoding/json"
	"io"
	"lunar_astral/adapters"
	"net/http"
)

func MessageBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "MessageBatch请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "MessageBatch请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req MessageBatchRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "MessageBatch请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	for _, msg := range req.Messages {
		adapters.UnreadContext = append(adapters.UnreadContext, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  len(adapters.UnreadContext),
	})
}

func VideoUrlBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "VideoUrlBatch请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "VideoUrlBatch请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req VideoUrlBatchRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "VideoUrlBatch请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	for _, url := range req.Urls {
		adapters.UnreadVideoUrl = append(adapters.UnreadVideoUrl, url)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  len(adapters.UnreadVideoUrl),
	})
}

// AgentPositionHandler 接收前端遥测数据，更新缓存的智能体3D位置
func AgentPositionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "AgentPosition请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "AgentPosition请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req AgentPositionRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "AgentPosition请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	adapters.UpdateAgentPosition(req.X, req.Y, req.Z)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  1,
	})
}

// AgentEventHandler 接收前端引擎事件，推送到 AI 上下文
func AgentEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "AgentEvent请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "AgentEvent请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req AgentEventRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "AgentEvent请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	adapters.PushAgentEventToContext(req.Event, req.Data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  1,
	})
}
