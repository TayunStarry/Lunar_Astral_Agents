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

// =============================================================================
// 记忆库端点 — 多集合扁平化 RESTful 架构
// 存储布局：<MemoryDBDir>/<collectionName>/{documents.json, metadata.json}
// URL 布局：/memory/<collectionName>/...（移除旧版 collections/ 中间层）
// =============================================================================

// MemoryHandler 记忆库统一分发器
// 支持路径：
//   POST   /memory/init                     实例初始化（配置嵌入服务连接）
//   GET    /memory/stats                    全局统计（聚合所有集合）
//   GET    /memory/collections              列出所有集合（保留字）
//   POST   /memory/{name}                   创建/打开集合（锁定模型）
//   GET    /memory/{name}/stats             集合统计
//   POST   /memory/{name}/messages          添加消息
//   GET    /memory/{name}/messages          查询消息
//   DELETE /memory/{name}/messages          删除消息
//   GET    /memory/{name}/documents         文档分页列表
//   POST   /memory/{name}/rebuild           重建（删除维度不符文档）
//
// 保留字：init、stats、collections 不可作为集合名
func MemoryHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/memory/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, "记忆库请求[ERROR] -> 路径不能为空")
		return
	}

	parts := strings.Split(path, "/")

	// 单段路径：保留字端点 或 集合创建
	if len(parts) == 1 {
		switch parts[0] {
		case "init":
			handleMemoryInit(w, r)
			return
		case "stats":
			handleMemoryGlobalStats(w, r)
			return
		case "collections":
			handleMemoryListCollections(w, r)
			return
		default:
			// /memory/{name} — 创建/打开集合
			handleMemoryCollectionCreate(w, r, parts[0])
			return
		}
	}

	// 两段路径：/memory/{name}/{action}
	collectionName := parts[0]
	action := parts[1]
	switch action {
	case "messages":
		handleMemoryMessages(w, r, collectionName)
	case "stats":
		handleMemoryCollectionStats(w, r, collectionName)
	case "documents":
		handleMemoryDocuments(w, r, collectionName)
	case "rebuild":
		handleMemoryRebuild(w, r, collectionName)
	default:
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未知的集合操作: %s", action))
	}
}

// handleMemoryInit POST /memory/init — 实例初始化（仅配置嵌入服务连接，不创建集合）
func handleMemoryInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if module.IsInitialized() {
		writeSuccess(w, map[string]string{
			"message": "记忆库实例已初始化",
		})
		return
	}

	var req memoryInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> base_url 不能为空")
		return
	}

	if !strings.HasPrefix(req.BaseURL, "http://") && !strings.HasPrefix(req.BaseURL, "https://") {
		req.BaseURL = "http://" + req.BaseURL
		logger.Warn("Storage", "记忆库 base_url 缺少协议前缀, 已自动补全为: %s", req.BaseURL)
	}

	if err := module.MemoryInitInstance(req.BaseURL, req.APIKey); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 实例初始化失败: %v", err))
		return
	}

	logger.Info("Storage", "记忆库实例初始化成功, base_url: %s", req.BaseURL)

	writeSuccess(w, map[string]string{
		"message":  "记忆库实例初始化成功",
		"base_url": req.BaseURL,
	})
}

// handleMemoryGlobalStats GET /memory/stats — 全局统计（聚合所有集合）
func handleMemoryGlobalStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	initialized := module.IsInitialized()
	if !initialized {
		writeSuccess(w, memoryStatsData{
			Initialized: false,
		})
		return
	}

	collections := module.MemoryListCollections()
	totalDocs := 0
	mismatch := false
	for _, name := range collections {
		totalDocs += module.GetCollectionCount(name)
		if module.HasSyncMismatch(name) {
			mismatch = true
		}
	}

	writeSuccess(w, memoryStatsData{
		DocumentCount: totalDocs,
		Initialized:   true,
		EntryCount:    totalDocs,
		SyncMismatch:  mismatch,
	})
}

// handleMemoryListCollections GET /memory/collections — 列出所有集合
func handleMemoryListCollections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	names := module.MemoryListCollections()
	infos := make([]memoryCollectionInfo, 0, len(names))
	for _, name := range names {
		model, dim, count, err := module.MemoryGetCollectionInfo(name)
		if err != nil {
			logger.Warn("Storage", "获取集合 [%s] 信息失败: %v", name, err)
			continue
		}
		infos = append(infos, memoryCollectionInfo{
			Name:      name,
			Model:     model,
			Dimension: dim,
			Count:     count,
		})
	}

	writeSuccess(w, map[string]interface{}{
		"collections": infos,
		"total":       len(infos),
	})
}

// handleMemoryCollectionCreate POST /memory/{name} — 创建/打开集合（探针定维度）
func handleMemoryCollectionCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化, 请先调用 /memory/init")
		return
	}

	var req memoryCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.ModelName == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> model_name 不能为空")
		return
	}

	ctx := context.Background()
	if err := module.CollectionInit(ctx, collectionName, req.ModelName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 集合创建失败: %v", err))
		return
	}

	model, dim, count, _ := module.MemoryGetCollectionInfo(collectionName)
	logger.Info("Storage", "集合 [%s] 创建成功, 模型: %s, 维度: %d, 文档数: %d",
		collectionName, model, dim, count)

	writeSuccess(w, memoryCollectionInfo{
		Name:      collectionName,
		Model:     model,
		Dimension: dim,
		Count:     count,
	})
}

// handleMemoryCollectionStats GET /memory/{name}/stats — 集合统计
func handleMemoryCollectionStats(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	model, dim, count, err := module.MemoryGetCollectionInfo(collectionName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> %v", err))
		return
	}

	mismatch := module.HasSyncMismatch(collectionName)

	logger.Info("Storage", "集合 [%s] 统计: 文档数=%d, 模型=%s, 维度=%d, 维度不符=%v",
		collectionName, count, model, dim, mismatch)

	writeSuccess(w, memoryStatsData{
		DocumentCount: count,
		Initialized:   true,
		EntryCount:    count,
		SyncMismatch:  mismatch,
	})
}

// handleMemoryMessages POST/GET/DELETE /memory/{name}/messages
func handleMemoryMessages(w http.ResponseWriter, r *http.Request, collectionName string) {
	switch r.Method {
	case http.MethodPost:
		handleMemoryAddMessage(w, r, collectionName)
	case http.MethodGet:
		handleMemoryQueryMessages(w, r, collectionName)
	case http.MethodDelete:
		handleMemoryDeleteMessage(w, r, collectionName)
	default:
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST/GET/DELETE")
	}
}

func handleMemoryAddMessage(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	var req memoryAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	if !slices.Contains(validRoles, req.Role) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 无效的角色: %s，仅支持 user/assistant/system", req.Role))
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 消息内容不能为空")
		return
	}

	ctx := context.Background()
	id, err := module.AddMessageWithID(ctx, collectionName, req.Role, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 添加消息失败: %v", err))
		return
	}

	logger.Info("Storage", "集合 [%s] 添加消息成功, ID: %s, 角色: %s, 内容长度: %d",
		collectionName, id, req.Role, len(req.Content))

	writeSuccess(w, map[string]string{
		"id":      id,
		"role":    req.Role,
		"content": req.Content,
	})
}

func handleMemoryQueryMessages(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	queryText := r.URL.Query().Get("query")
	if queryText == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 查询文本不能为空")
		return
	}

	topK := 10
	if topKStr := r.URL.Query().Get("top_k"); topKStr != "" {
		if val, err := strconv.Atoi(topKStr); err == nil && val > 0 && val <= 100 {
			topK = val
		}
	}

	ctx := context.Background()
	messages, err := module.QueryMessagesWithContent(ctx, collectionName, queryText, topK)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 查询失败: %v", err))
		return
	}

	results := make([]memoryMessageData, 0, len(messages))
	for _, msg := range messages {
		results = append(results, memoryMessageData{
			ID:         msg.ID,
			Role:       msg.Role,
			Content:    msg.Content,
			Similarity: msg.Similarity,
		})
	}

	logger.Info("Storage", "集合 [%s] 查询完成, 查询: %s, 结果数: %d",
		collectionName, queryText, len(results))

	writeSuccess(w, memoryQueryData{
		Query:      queryText,
		TopK:       topK,
		Results:    results,
		TotalFound: len(results),
	})
}

func handleMemoryDeleteMessage(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	var req memoryDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 消息ID不能为空")
		return
	}

	ctx := context.Background()
	if err := module.DeleteMessage(ctx, collectionName, req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 删除消息失败: %v", err))
		return
	}

	logger.Info("Storage", "集合 [%s] 删除消息成功, ID: %s", collectionName, req.ID)

	writeSuccess(w, map[string]string{
		"id": req.ID,
	})
}

// handleMemoryDocuments GET /memory/{name}/documents — 文档分页列表
func handleMemoryDocuments(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
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

	entries, total := module.GetDocuments(collectionName, offset, limit)

	docList := make([]memoryMessageData, 0, len(entries))
	for _, entry := range entries {
		docList = append(docList, memoryMessageData{
			ID:      entry.ID,
			Role:    entry.Role,
			Content: entry.Content,
		})
	}

	writeSuccess(w, map[string]interface{}{
		"documents": docList,
		"total":     total,
		"offset":    offset,
		"limit":     limit,
	})
}

// handleMemoryRebuild POST /memory/{name}/rebuild — 重建（删除维度不符文档）
func handleMemoryRebuild(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	ctx := context.Background()
	count, err := module.RebuildEntries(ctx, collectionName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 重建条目失败: %v", err))
		return
	}

	logger.Info("Storage", "集合 [%s] rebuild 完成, 剩余 %d 条文档", collectionName, count)

	writeSuccess(w, map[string]interface{}{
		"rebuilt": count,
		"message": fmt.Sprintf("集合 [%s] 重建完成, 剩余 %d 条文档", collectionName, count),
	})
}
