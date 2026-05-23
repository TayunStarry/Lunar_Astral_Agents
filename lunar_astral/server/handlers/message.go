package handlers

import (
	"lunar_astral/adapters"
	"encoding/json"
	"io"
	"net/http"
)

type MessageBatchRequest struct {
	Messages []adapters.PostMessage `json:"messages"`
}

type VideoUrlBatchRequest struct {
	Urls []string `json:"urls"`
}

type BatchResponse struct {
	Success bool `json:"success"`
	Length  int  `json:"length"`
}

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
