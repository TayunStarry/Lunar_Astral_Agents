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

func writeJSON(w http.ResponseWriter, statusCode int, resp map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// =============================================================================
// 数据库操作端点 — 替代 database.go
// =============================================================================

func DatabaseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "数据库请求[ERROR] -> 不允许的请求方法")
		return
	}

	var req module.DatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("数据库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	result := module.ExecuteDatabaseRequest(req)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("数据库请求[ERROR] -> 编码响应失败: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Info("Storage", "数据库批量操作成功，执行 %d 个操作，耗时 %dms", result.Operations, result.TotalTime)
}

// =============================================================================
// 向量数据库端点 — 多集合 RESTful 架构
// 路由：/vector/ 子树分发，解析路径中的集合名
// =============================================================================

// VectorHandler 向量数据库统一分发器
// 支持路径：
//   POST   /vector/init                            实例初始化（配置嵌入服务连接）
//   GET    /vector/stats                           全局统计（聚合所有集合）
//   GET    /vector/collections                     列出所有集合
//   POST   /vector/collections/{name}              创建/打开集合（锁定模型）
//   GET    /vector/collections/{name}/stats        集合统计
//   POST   /vector/collections/{name}/messages     添加消息
//   GET    /vector/collections/{name}/messages     查询消息
//   DELETE /vector/collections/{name}/messages     删除消息
//   GET    /vector/collections/{name}/documents    文档分页列表
//   POST   /vector/collections/{name}/rebuild      重建（删除维度不符文档）
func VectorHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/vector/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, "向量数据库请求[ERROR] -> 路径不能为空")
		return
	}

	parts := strings.Split(path, "/")

	// /vector/init
	if len(parts) == 1 && parts[0] == "init" {
		handleVectorInit(w, r)
		return
	}

	// /vector/stats
	if len(parts) == 1 && parts[0] == "stats" {
		handleVectorGlobalStats(w, r)
		return
	}

	// /vector/collections 或 /vector/collections/{name}/...
	if parts[0] != "collections" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("向量数据库请求[ERROR] -> 未知路径: /vector/%s", path))
		return
	}

	// /vector/collections — 列出所有集合
	if len(parts) == 1 {
		handleVectorListCollections(w, r)
		return
	}

	// parts[1] = 集合名, parts[2+] = 操作
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "向量数据库请求[ERROR] -> 集合名不能为空")
		return
	}

	collectionName := parts[1]

	// /vector/collections/{name} — 创建/打开集合
	if len(parts) == 2 {
		handleVectorCollectionCreate(w, r, collectionName)
		return
	}

	action := parts[2]
	switch action {
	case "messages":
		handleVectorMessages(w, r, collectionName)
	case "stats":
		handleVectorCollectionStats(w, r, collectionName)
	case "documents":
		handleVectorDocuments(w, r, collectionName)
	case "rebuild":
		handleVectorRebuild(w, r, collectionName)
	default:
		writeError(w, http.StatusNotFound, fmt.Sprintf("向量数据库请求[ERROR] -> 未知的集合操作: %s", action))
	}
}

// handleVectorInit POST /vector/init — 实例初始化（仅配置嵌入服务连接，不创建集合）
func handleVectorInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if module.IsInitialized() {
		writeSuccess(w, map[string]string{
			"message": "向量数据库实例已初始化",
		})
		return
	}

	var req vectorInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "向量数据库请求[ERROR] -> base_url 不能为空")
		return
	}

	if !strings.HasPrefix(req.BaseURL, "http://") && !strings.HasPrefix(req.BaseURL, "https://") {
		req.BaseURL = "http://" + req.BaseURL
		logger.Warn("Storage", "向量 base_url 缺少协议前缀, 已自动补全为: %s", req.BaseURL)
	}

	if err := module.VectorInitInstance(req.BaseURL, req.APIKey); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("向量数据库请求[ERROR] -> 实例初始化失败: %v", err))
		return
	}

	logger.Info("Storage", "向量实例初始化成功, base_url: %s", req.BaseURL)

	writeSuccess(w, map[string]string{
		"message":  "向量数据库实例初始化成功",
		"base_url": req.BaseURL,
	})
}

// handleVectorGlobalStats GET /vector/stats — 全局统计（聚合所有集合）
func handleVectorGlobalStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	initialized := module.IsInitialized()
	if !initialized {
		writeSuccess(w, vectorStatsData{
			Initialized: false,
		})
		return
	}

	collections := module.VectorListCollections()
	totalDocs := 0
	mismatch := false
	for _, name := range collections {
		totalDocs += module.GetCollectionCount(name)
		if module.HasSyncMismatch(name) {
			mismatch = true
		}
	}

	writeSuccess(w, vectorStatsData{
		DocumentCount: totalDocs,
		Initialized:   true,
		EntryCount:    totalDocs,
		SyncMismatch:  mismatch,
	})
}

// handleVectorListCollections GET /vector/collections — 列出所有集合
func handleVectorListCollections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
		return
	}

	names := module.VectorListCollections()
	infos := make([]vectorCollectionInfo, 0, len(names))
	for _, name := range names {
		model, dim, count, err := module.VectorGetCollectionInfo(name)
		if err != nil {
			logger.Warn("Storage", "获取集合 [%s] 信息失败: %v", name, err)
			continue
		}
		infos = append(infos, vectorCollectionInfo{
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

// handleVectorCollectionCreate POST /vector/collections/{name} — 创建/打开集合（探针定维度）
func handleVectorCollectionCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化, 请先调用 /vector/init")
		return
	}

	var req vectorCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.ModelName == "" {
		writeError(w, http.StatusBadRequest, "向量数据库请求[ERROR] -> model_name 不能为空")
		return
	}

	ctx := context.Background()
	if err := module.CollectionInit(ctx, collectionName, req.ModelName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("向量数据库请求[ERROR] -> 集合创建失败: %v", err))
		return
	}

	model, dim, count, _ := module.VectorGetCollectionInfo(collectionName)
	logger.Info("Storage", "集合 [%s] 创建成功, 模型: %s, 维度: %d, 文档数: %d",
		collectionName, model, dim, count)

	writeSuccess(w, vectorCollectionInfo{
		Name:      collectionName,
		Model:     model,
		Dimension: dim,
		Count:     count,
	})
}

// handleVectorCollectionStats GET /vector/collections/{name}/stats — 集合统计
func handleVectorCollectionStats(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
		return
	}

	model, dim, count, err := module.VectorGetCollectionInfo(collectionName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("向量数据库请求[ERROR] -> %v", err))
		return
	}

	mismatch := module.HasSyncMismatch(collectionName)

	logger.Info("Storage", "集合 [%s] 统计: 文档数=%d, 模型=%s, 维度=%d, 维度不符=%v",
		collectionName, count, model, dim, mismatch)

	writeSuccess(w, vectorStatsData{
		DocumentCount: count,
		Initialized:   true,
		EntryCount:    count,
		SyncMismatch:  mismatch,
	})
}

// handleVectorMessages POST/GET/DELETE /vector/collections/{name}/messages
func handleVectorMessages(w http.ResponseWriter, r *http.Request, collectionName string) {
	switch r.Method {
	case http.MethodPost:
		handleVectorAddMessage(w, r, collectionName)
	case http.MethodGet:
		handleVectorQueryMessages(w, r, collectionName)
	case http.MethodDelete:
		handleVectorDeleteMessage(w, r, collectionName)
	default:
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST/GET/DELETE")
	}
}

func handleVectorAddMessage(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
		return
	}

	var req vectorAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	if !slices.Contains(validRoles, req.Role) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("向量数据库请求[ERROR] -> 无效的角色: %s，仅支持 user/assistant/system", req.Role))
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "向量数据库请求[ERROR] -> 消息内容不能为空")
		return
	}

	ctx := context.Background()
	id, err := module.AddMessageWithID(ctx, collectionName, req.Role, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("向量数据库请求[ERROR] -> 添加消息失败: %v", err))
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

func handleVectorQueryMessages(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
		return
	}

	queryText := r.URL.Query().Get("query")
	if queryText == "" {
		writeError(w, http.StatusBadRequest, "向量数据库请求[ERROR] -> 查询文本不能为空")
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("向量数据库请求[ERROR] -> 查询失败: %v", err))
		return
	}

	results := make([]vectorMessageData, 0, len(messages))
	for _, msg := range messages {
		results = append(results, vectorMessageData{
			ID:         msg.ID,
			Role:       msg.Role,
			Content:    msg.Content,
			Similarity: msg.Similarity,
		})
	}

	logger.Info("Storage", "集合 [%s] 查询完成, 查询: %s, 结果数: %d",
		collectionName, queryText, len(results))

	writeSuccess(w, vectorQueryData{
		Query:      queryText,
		TopK:       topK,
		Results:    results,
		TotalFound: len(results),
	})
}

func handleVectorDeleteMessage(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
		return
	}

	var req vectorDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("向量数据库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "向量数据库请求[ERROR] -> 消息ID不能为空")
		return
	}

	ctx := context.Background()
	if err := module.DeleteMessage(ctx, collectionName, req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("向量数据库请求[ERROR] -> 删除消息失败: %v", err))
		return
	}

	logger.Info("Storage", "集合 [%s] 删除消息成功, ID: %s", collectionName, req.ID)

	writeSuccess(w, map[string]string{
		"id": req.ID,
	})
}

// handleVectorDocuments GET /vector/collections/{name}/documents — 文档分页列表
func handleVectorDocuments(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
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

	docList := make([]vectorMessageData, 0, len(entries))
	for _, entry := range entries {
		docList = append(docList, vectorMessageData{
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

// handleVectorRebuild POST /vector/collections/{name}/rebuild — 重建（删除维度不符文档）
func handleVectorRebuild(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "向量数据库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsInitialized() {
		writeError(w, http.StatusServiceUnavailable, "向量数据库请求[ERROR] -> 向量数据库未初始化")
		return
	}

	ctx := context.Background()
	count, err := module.RebuildEntries(ctx, collectionName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("向量数据库请求[ERROR] -> 重建条目失败: %v", err))
		return
	}

	logger.Info("Storage", "集合 [%s] rebuild 完成, 剩余 %d 条文档", collectionName, count)

	writeSuccess(w, map[string]interface{}{
		"rebuilt": count,
		"message": fmt.Sprintf("集合 [%s] 重建完成, 剩余 %d 条文档", collectionName, count),
	})
}
