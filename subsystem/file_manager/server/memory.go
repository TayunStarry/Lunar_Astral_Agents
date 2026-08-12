package server

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// =============================================================================
// v2 记忆库端点 — 统一 text/image 架构，标签向量中介检索
// 存储布局：<MemoryDBDir>/<collectionName>/{metadata.json, documents_*.json, images_*.json, tags_*.json}
// URL 布局：/memory/<collectionName>/...（移除了旧版 collections/ 中间层和 images 专用端点）
// =============================================================================

// MemoryHandler 记忆库统一分发器
// 支持路径：
//
//	POST   /memory/init                     实例初始化（嵌入服务 + LLM 服务）
//	GET    /memory/stats                    全局统计（聚合所有集合）
//	GET    /memory/collections              列出所有集合（保留字）
//	POST   /memory/{name}                   创建/打开集合（锁定模型）
//	DELETE /memory/{name}                   删除集合（移除目录及所有文档）
//	GET    /memory/{name}/stats             集合统计
//	POST   /memory/{name}/messages          添加消息/图片（统一端点）
//	GET    /memory/{name}/messages          查询消息/图片（统一端点）
//	DELETE /memory/{name}/messages          删除消息/图片（统一端点）
//	GET    /memory/{name}/documents         文档分页列表
//	POST   /memory/{name}/rebuild           重建标签向量
//	POST   /memory/{name}/clear             清空集合
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

	// 单段路径：保留字端点 或 集合操作
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
			// DELETE /memory/{name} — 删除集合
			if r.Method == http.MethodDelete {
				handleMemoryDeleteCollection(w, r, parts[0])
				return
			}
			// POST /memory/{name} — 创建/打开集合
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
	case "clear":
		handleMemoryClearCollection(w, r, collectionName)
	default:
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未知的集合操作: %s", action))
	}
}

// handleMemoryInit POST /memory/init — v2 实例初始化（嵌入服务 + LLM 标签生成服务）
// 模型配置从 config 模块（lunar_config.json）读取，不再通过请求体传入
func handleMemoryInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if module.IsMemoryInitialized() {
		writeSuccess(w, map[string]string{
			"message": "记忆库实例已初始化",
		})
		return
	}

	if err := module.MemoryInitInstance(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 实例初始化失败: %v", err))
		return
	}

	LoggerGeneral.Info("FileManager", "记忆库实例初始化成功（模型配置从 lunar_config.json 读取）")

	writeSuccess(w, map[string]string{
		"message": "记忆库实例初始化成功",
	})
}

// handleMemoryGlobalStats GET /memory/stats — 全局统计（聚合所有集合）
func handleMemoryGlobalStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	initialized := module.IsMemoryInitialized()
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
		totalDocs += module.MemoryGetEntryCount(name)
		if module.MemoryHasSyncMismatch(name) {
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

	// 集合列表是纯磁盘扫描操作，无需嵌入服务初始化即可工作
	names := module.MemoryListCollections()
	infos := make([]memoryCollectionInfo, 0, len(names))
	for _, name := range names {
		info := module.MemoryGetCollectionInfoWithType(name)
		if info == nil {
			continue
		}
		infos = append(infos, memoryCollectionInfo{
			Name:            name,
			EmbeddingModel:  getStringField(info, "embedding_model"),
			Dimension:       getIntField(info, "embedding_dimension"),
			Count:           getIntField(info, "document_count"),
			Type:            getStringField(info, "type"),
			MultimodalModel: getStringField(info, "multimodal_model"),
			Version:         getIntField(info, "version"),
			TagCount:        getIntField(info, "tag_count"),
		})
	}

	writeSuccess(w, map[string]interface{}{
		"collections": infos,
		"total":       len(infos),
	})
}

// handleMemoryCollectionCreate POST /memory/{name} — v2 创建/打开集合
func handleMemoryCollectionCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsMemoryInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化, 请先调用 /memory/init")
		return
	}

	var req memoryCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	// 模型名从 config 模块（lunar_config.json memory.embedding_model）读取
	modelName := *GeneralConfig.MemoryEmbeddingModel

	collType := req.CollectionType
	if collType == "" {
		collType = module.CollectionTypeText
	}
	if collType != module.CollectionTypeText && collType != module.CollectionTypeImage {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 无效的集合类型: %s，仅支持 text/image", collType))
		return
	}

	ctx := context.Background()
	if err := module.CollectionInit(ctx, collectionName, modelName, collType); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 集合创建失败: %v", err))
		return
	}

	info := module.MemoryGetCollectionInfoWithType(collectionName)
	LoggerGeneral.Info("FileManager", "集合 [%s] 创建成功, 类型: %s, 模型: %s, 维度: %d",
		collectionName, collType, modelName, getIntField(info, "embedding_dimension"))

	writeSuccess(w, memoryCollectionInfo{
		Name:           collectionName,
		EmbeddingModel: modelName,
		Dimension:      getIntField(info, "embedding_dimension"),
		Count:          getIntField(info, "document_count"),
		Type:           collType,
		TagCount:       getIntField(info, "tag_count"),
	})
}

// handleMemoryCollectionStats GET /memory/{name}/stats — 集合统计
func handleMemoryCollectionStats(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	// 集合统计是纯磁盘读取操作，无需嵌入服务初始化即可工作
	info := module.MemoryGetCollectionInfo(collectionName)
	if info == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 集合 '%s' 不存在", collectionName))
		return
	}

	count := getIntField(info, "document_count")
	mismatch := module.MemoryHasSyncMismatch(collectionName)

	LoggerGeneral.Info("FileManager", "集合 [%s] 统计: 文档数=%d, 标签数=%d, 维度不符=%v",
		collectionName, count, getIntField(info, "tag_count"), mismatch)

	writeSuccess(w, memoryStatsData{
		DocumentCount: count,
		Initialized:   true,
		EntryCount:    count,
		SyncMismatch:  mismatch,
	})
}

// handleMemoryMessages POST/GET/DELETE /memory/{name}/messages — v2 统一消息/图片端点
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

// handleMemoryAddMessage v2 统一添加端点（text 和 image 共用）
func handleMemoryAddMessage(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsMemoryInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	var req memoryAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	ctx := context.Background()

	if req.Image != "" {
		// 图片文档添加
		id, err := module.MemoryAddImage(ctx, collectionName, req.Image)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 添加图片失败: %v", err))
			return
		}
		LoggerGeneral.Info("FileManager", "集合 [%s] 添加图片成功, ID: %s", collectionName, id)
		writeSuccess(w, map[string]string{"id": id, "type": "image"})
	} else {
		// 文本文档添加
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

		id, err := module.MemoryAddMessage(ctx, collectionName, req.Role, req.Content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 添加消息失败: %v", err))
			return
		}

		LoggerGeneral.Info("FileManager", "集合 [%s] 添加消息成功, ID: %s, 角色: %s, 内容长度: %d",
			collectionName, id, req.Role, len(req.Content))

		writeSuccess(w, map[string]string{
			"id":      id,
			"role":    req.Role,
			"content": req.Content,
		})
	}
}

// handleMemoryQueryMessages v2 统一查询端点
func handleMemoryQueryMessages(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsMemoryInitialized() {
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
	messages, err := module.MemoryQueryMessagesWithContent(ctx, collectionName, queryText, topK)
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
			Image:      msg.Image,
			Similarity: msg.Similarity,
		})
	}

	LoggerGeneral.Info("FileManager", "集合 [%s] 查询完成, 查询: %s, 结果数: %d",
		collectionName, queryText, len(results))

	writeSuccess(w, memoryQueryData{
		Query:      queryText,
		TopK:       topK,
		Results:    results,
		TotalFound: len(results),
	})
}

// handleMemoryDeleteMessage v2 统一删除端点
func handleMemoryDeleteMessage(w http.ResponseWriter, r *http.Request, collectionName string) {
	if !module.IsMemoryInitialized() {
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
	if err := module.MemoryDeleteMessage(ctx, collectionName, req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 删除消息失败: %v", err))
		return
	}

	LoggerGeneral.Info("FileManager", "集合 [%s] 删除消息成功, ID: %s", collectionName, req.ID)

	writeSuccess(w, map[string]string{
		"id": req.ID,
	})
}

// handleMemoryDocuments GET /memory/{name}/documents — v2 文档分页列表
func handleMemoryDocuments(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	// 文档列表是纯磁盘读取操作，无需嵌入服务初始化即可工作
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

	entries, total := module.MemoryGetDocuments(collectionName, offset, limit)

	docList := make([]memoryMessageData, 0, len(entries))
	for _, entry := range entries {
		docList = append(docList, memoryMessageData{
			ID:      entry.ID,
			Role:    entry.Role,
			Content: entry.Content,
			Image:   entry.Image,
		})
	}

	writeSuccess(w, map[string]interface{}{
		"documents": docList,
		"total":     total,
		"offset":    offset,
		"limit":     limit,
	})
}

// handleMemoryRebuild POST /memory/{name}/rebuild — v2 重建标签向量
func handleMemoryRebuild(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsMemoryInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	ctx := context.Background()
	if err := module.MemoryRebuildEntries(ctx, collectionName, "", nil); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 重建条目失败: %v", err))
		return
	}

	count := module.MemoryGetEntryCount(collectionName)
	LoggerGeneral.Info("FileManager", "集合 [%s] rebuild 完成, 剩余 %d 条文档", collectionName, count)

	writeSuccess(w, map[string]interface{}{
		"rebuilt": count,
		"message": fmt.Sprintf("集合 [%s] 重建完成, 剩余 %d 条文档", collectionName, count),
	})
}

// handleMemoryDeleteCollection DELETE /memory/{name} — 删除集合
func handleMemoryDeleteCollection(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 DELETE")
		return
	}

	if !module.IsMemoryInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	if err := module.MemoryDeleteCollection(collectionName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 删除集合失败: %v", err))
		return
	}

	LoggerGeneral.Info("FileManager", "集合 [%s] 删除成功", collectionName)

	writeSuccess(w, map[string]string{
		"message": fmt.Sprintf("集合 [%s] 已删除", collectionName),
	})
}

// handleMemoryClearCollection POST /memory/{name}/clear — 清空集合
func handleMemoryClearCollection(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !module.IsMemoryInitialized() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	if err := module.MemoryClearCollection(collectionName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 清空集合失败: %v", err))
		return
	}

	LoggerGeneral.Info("FileManager", "集合 [%s] 清空成功", collectionName)

	writeSuccess(w, map[string]string{
		"message": fmt.Sprintf("集合 [%s] 已清空", collectionName),
	})
}

// =============================================================================
// 辅助函数
// =============================================================================

// getStringField 从 map[string]interface{} 安全获取字符串字段
func getStringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getIntField 从 map[string]interface{} 安全获取整数字段
func getIntField(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		}
	}
	return 0
}
