package server

import (
	"context"
	"encoding/json"
	"fmt"
	"logger"
	"net/http"
	"slices"
	"storage/module"
	"strconv"
	"strings"
)

var validRoles = []string{"user", "assistant", "system"}

type chromemAddRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chromemDeleteRequest struct {
	ID string `json:"id"`
}

type chromemResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type chromemStatsData struct {
	DocumentCount int  `json:"document_count"`
	Initialized   bool `json:"initialized"`
	EntryCount    int  `json:"entry_count"`
	SyncMismatch  bool `json:"sync_mismatch"`
}

type chromemMessageData struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chromemQueryData struct {
	Query      string               `json:"query"`
	TopK       int                  `json:"top_k"`
	Results    []chromemMessageData `json:"results"`
	TotalFound int                  `json:"total_found"`
}

func writeChromemJSON(w http.ResponseWriter, statusCode int, resp chromemResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

type chromemInitRequest struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	ModelName string `json:"model_name"`
}

func ChromemMessagesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleChromemAdd(w, r)
	case http.MethodGet:
		handleChromemQuery(w, r)
	case http.MethodDelete:
		handleChromemDelete(w, r)
	default:
		writeChromemJSON(w, http.StatusMethodNotAllowed, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST/GET/DELETE",
		})
	}
}

func handleChromemAdd(w http.ResponseWriter, r *http.Request) {
	if !module.IsInitialized() {
		writeChromemJSON(w, http.StatusServiceUnavailable, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> chromem 未初始化",
		})
		return
	}

	var req chromemAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err),
		})
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	if !slices.Contains(validRoles, req.Role) {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 无效的角色: %s，仅支持 user/assistant/system", req.Role),
		})
		return
	}

	if req.Content == "" {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 消息内容不能为空",
		})
		return
	}

	ctx := context.Background()
	id, err := module.AddMessageWithID(ctx, req.Role, req.Content)
	if err != nil {
		writeChromemJSON(w, http.StatusInternalServerError, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 添加消息失败: %v", err),
		})
		return
	}

	logger.Info("Storage", "chromem 添加消息成功, ID: %s, 角色: %s, 内容长度: %d", id, req.Role, len(req.Content))

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: map[string]string{
			"id":      id,
			"role":    req.Role,
			"content": req.Content,
		},
	})
}

func handleChromemQuery(w http.ResponseWriter, r *http.Request) {
	if !module.IsInitialized() {
		writeChromemJSON(w, http.StatusServiceUnavailable, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> chromem 未初始化",
		})
		return
	}

	queryText := r.URL.Query().Get("query")
	if queryText == "" {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 查询文本不能为空",
		})
		return
	}

	topK := 10
	if topKStr := r.URL.Query().Get("top_k"); topKStr != "" {
		if val, err := strconv.Atoi(topKStr); err == nil && val > 0 && val <= 100 {
			topK = val
		}
	}

	ctx := context.Background()
	messages, err := module.QueryMessagesWithContent(ctx, queryText, topK)
	if err != nil {
		writeChromemJSON(w, http.StatusInternalServerError, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 查询失败: %v", err),
		})
		return
	}

	results := make([]chromemMessageData, 0, len(messages))
	for _, msg := range messages {
		results = append(results, chromemMessageData{
			ID:      msg["id"],
			Role:    msg["role"],
			Content: msg["content"],
		})
	}

	logger.Info("Storage", "chromem 查询完成, 查询: %s, 结果数: %d", queryText, len(results))

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: chromemQueryData{
			Query:      queryText,
			TopK:       topK,
			Results:    results,
			TotalFound: len(results),
		},
	})
}

func handleChromemDelete(w http.ResponseWriter, r *http.Request) {
	if !module.IsInitialized() {
		writeChromemJSON(w, http.StatusServiceUnavailable, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> chromem 未初始化",
		})
		return
	}

	var req chromemDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err),
		})
		return
	}

	if req.ID == "" {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 消息ID不能为空",
		})
		return
	}

	ctx := context.Background()
	if err := module.DeleteMessage(ctx, req.ID); err != nil {
		writeChromemJSON(w, http.StatusInternalServerError, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 删除消息失败: %v", err),
		})
		return
	}

	logger.Info("Storage", "chromem 删除消息成功, ID: %s", req.ID)

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: map[string]string{
			"id": req.ID,
		},
	})
}

func ChromemStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeChromemJSON(w, http.StatusMethodNotAllowed, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 GET",
		})
		return
	}

	count := module.GetCollectionCount()
	initialized := module.IsInitialized()
	entryCount := module.GetEntryCount()
	mismatch := module.HasSyncMismatch()

	logger.Info("Storage", "chromem 统计信息: 文档数=%d, 已初始化=%v, 条目数=%d, 不同步=%v", count, initialized, entryCount, mismatch)

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: chromemStatsData{
			DocumentCount: count,
			Initialized:   initialized,
			EntryCount:    entryCount,
			SyncMismatch:  mismatch,
		},
	})
}

func ChromemInitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeChromemJSON(w, http.StatusMethodNotAllowed, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST",
		})
		return
	}

	if module.IsInitialized() {
		writeChromemJSON(w, http.StatusOK, chromemResponse{
			Success: true,
			Data: map[string]string{
				"message": "chromem 向量数据库已初始化",
			},
		})
		return
	}

	var req chromemInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err),
		})
		return
	}

	if req.BaseURL == "" {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> base_url 不能为空",
		})
		return
	}

	if !strings.HasPrefix(req.BaseURL, "http://") && !strings.HasPrefix(req.BaseURL, "https://") {
		req.BaseURL = "http://" + req.BaseURL
		logger.Warn("Storage", "chromem base_url 缺少协议前缀, 已自动补全为: %s", req.BaseURL)
	}

	if req.ModelName == "" {
		writeChromemJSON(w, http.StatusBadRequest, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> model_name 不能为空",
		})
		return
	}

	if err := module.Init(req.BaseURL, req.APIKey, req.ModelName); err != nil {
		writeChromemJSON(w, http.StatusInternalServerError, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 初始化失败: %v", err),
		})
		return
	}

	logger.Info("Storage", "chromem 初始化成功, base_url: %s, model: %s", req.BaseURL, req.ModelName)

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: map[string]string{
			"message":    "chromem 向量数据库初始化成功",
			"base_url":   req.BaseURL,
			"model_name": req.ModelName,
		},
	})
}

func ChromemDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeChromemJSON(w, http.StatusMethodNotAllowed, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 GET",
		})
		return
	}

	if !module.IsInitialized() {
		writeChromemJSON(w, http.StatusServiceUnavailable, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> chromem 未初始化",
		})
		return
	}

	offset := 0
	if offStr := r.URL.Query().Get("offset"); offStr != "" {
		if val, err := strconv.Atoi(offStr); err == nil && val >= 0 {
			offset = val
		}
	}

	limit := 20
	if limStr := r.URL.Query().Get("limit"); limStr != "" {
		if val, err := strconv.Atoi(limStr); err == nil && val > 0 && val <= 100 {
			limit = val
		}
	}

	entries, total := module.GetDocuments(offset, limit)

	docList := make([]chromemMessageData, 0, len(entries))
	for _, entry := range entries {
		docList = append(docList, chromemMessageData{
			ID:      entry.ID,
			Role:    entry.Role,
			Content: entry.Content,
		})
	}

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: map[string]interface{}{
			"documents": docList,
			"total":     total,
			"offset":    offset,
			"limit":     limit,
		},
	})
}

func ChromemRebuildHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeChromemJSON(w, http.StatusMethodNotAllowed, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST",
		})
		return
	}

	if !module.IsInitialized() {
		writeChromemJSON(w, http.StatusServiceUnavailable, chromemResponse{
			Success: false,
			Error:   "向量数据库请求[ERROR] -> chromem 未初始化",
		})
		return
	}

	ctx := context.Background()
	count, err := module.RebuildEntries(ctx)
	if err != nil {
		writeChromemJSON(w, http.StatusInternalServerError, chromemResponse{
			Success: false,
			Error:   fmt.Sprintf("向量数据库请求[ERROR] -> 重建条目失败: %v", err),
		})
		return
	}

	logger.Info("Storage", "chromem rebuild 完成, 重建 %d 条文档条目", count)

	writeChromemJSON(w, http.StatusOK, chromemResponse{
		Success: true,
		Data: map[string]interface{}{
			"rebuilt": count,
			"message": fmt.Sprintf("成功重建 %d 条文档条目", count),
		},
	})
}
