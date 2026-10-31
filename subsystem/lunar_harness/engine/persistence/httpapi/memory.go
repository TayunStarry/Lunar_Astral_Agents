package httpapi

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// 记忆库端点 — 集合是内容自动归入的分组，查询只按类型（text/image/audio）
//
// 存储布局：<MemoryDBDir>/<collectionName>/{metadata.json, documents_*.json, embeddings_*.json}
// 查询/写入不再按集合指定，集合名只出现在管理端点中。
//
// URL 布局：
//
//	POST   /memory/init                 实例初始化（加载全部集合、补齐默认分组与集合名向量）
//	GET    /memory/stats                全局统计（按类型聚合）
//	GET    /memory/collections          列出全部分组集合
//	POST   /memory/collections          创建分组集合（body: {"name":"..."}）
//	GET    /memory/messages             全局检索（query + type + top_k）
//	POST   /memory/messages             添加记忆（type=text/image/audio，内容自动归组）
//	DELETE /memory/messages             按 id 删除记忆
//	GET    /memory/entries              逐条列举（id=按 ID；type/offset/limit；limit=0 全部）
//	POST   /memory/fuse                 权重融合（把集合内超高相似度的文本条目合并为一条并重算向量）
//	GET    /memory/{name}/stats         集合统计
//	GET    /memory/{name}/documents     集合内文档分页（管理用途）
//	POST   /memory/{name}/rebuild       重建集合内容向量
//	POST   /memory/{name}/reload        外部改写分片后刷新内存缓存（不重算向量）
//	POST   /memory/{name}/clear         清空集合
// =============================================================================

// MemoryHandler 记忆库统一分发器
func MemoryHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/memory/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, http.StatusNotFound, "记忆库请求[ERROR] -> 路径不能为空")
		return
	}

	parts := strings.Split(path, "/")

	// 单段路径：全局端点，或旧版集合创建/删除
	if len(parts) == 1 {
		switch parts[0] {
		case "init":
			handleMemoryInit(w, r)
			return
		case "stats":
			handleMemoryGlobalStats(w, r)
			return
		case "collections":
			if r.Method == http.MethodPost {
				handleMemoryCreateCollection(w, r)
				return
			}
			handleMemoryListCollections(w, r)
			return
		case "messages":
			handleMemoryMessages(w, r)
			return
		case "entries":
			handleMemoryEntries(w, r)
			return
		case "fuse":
			handleMemoryFuse(w, r)
			return
		default:
			writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未知的记忆库端点: %s", parts[0]))
			return
		}
	}

	// 两段路径：/memory/{name}/{action}
	collectionName := parts[0]
	action := parts[1]
	switch action {
	case "messages":
		writeError(w, http.StatusGone, "记忆库请求[ERROR] -> 已不再按集合读写记忆，请改用 /memory/messages（type 指定返回类型）")
	case "stats":
		handleMemoryCollectionStats(w, r, collectionName)
	case "documents":
		handleMemoryDocuments(w, r, collectionName)
	case "rebuild":
		handleMemoryRebuild(w, r, collectionName)
	case "reload":
		handleMemoryReloadCollection(w, r, collectionName)
	case "clear":
		handleMemoryClearCollection(w, r, collectionName)
	default:
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未知的集合操作: %s", action))
	}
}

// handleMemoryInit POST /memory/init — 实例初始化（全部集合 + 默认分组 + 集合名向量）
func handleMemoryInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if err := memory.EnsureReady(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 实例初始化失败: %v", err))
		return
	}

	logger.Info("FileManager", "记忆库实例初始化成功（全部集合已加载，默认分组与集合名向量已就绪）")

	writeSuccess(w, map[string]string{
		"message": "记忆库实例初始化成功",
	})
}

// handleMemoryGlobalStats GET /memory/stats — 全局统计（按类型聚合）
func handleMemoryGlobalStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	if !memory.Ready() {
		writeSuccess(w, memoryStatsData{Initialized: false})
		return
	}

	totalDocs := 0
	mismatch := false
	for _, name := range memory.Collections() {
		totalDocs += memory.DocCount(name)
		if memory.HasSyncMismatch(name) {
			mismatch = true
		}
	}

	writeSuccess(w, memoryStatsData{
		DocumentCount: totalDocs,
		Initialized:   true,
		EntryCount:    totalDocs,
		SyncMismatch:  mismatch,
		TextCount:     memory.Count(memory.TypeText),
		ImageCount:    memory.Count(memory.TypeImage),
		AudioCount:    memory.Count(memory.TypeAudio),
	})
}

// handleMemoryListCollections GET /memory/collections — 列出全部分组集合
func handleMemoryListCollections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	names := memory.Collections()
	infos := make([]memoryCollectionInfo, 0, len(names))
	for _, name := range names {
		info := memory.CollectionInfo(name)
		if info == nil {
			continue
		}
		infos = append(infos, memoryCollectionInfo{
			Name:                   name,
			Dimension:              getIntField(info, "embedding_dimension"),
			Count:                  getIntField(info, "document_count"),
			Version:                getIntField(info, "version"),
			HasNameEmbedding:       getBoolField(info, "has_name_embedding"),
			NameEmbeddingDimension: getIntField(info, "name_embedding_dimension"),
			NameEmbeddingModel:     getStringField(info, "name_embedding_model"),
		})
	}

	writeSuccess(w, map[string]interface{}{
		"collections": infos,
		"total":       len(infos),
	})
}

// handleMemoryCreateCollection POST /memory/collections — 创建集合
func handleMemoryCreateCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	var req memoryCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}
	collectionName := strings.TrimSpace(req.Name)
	if collectionName == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 集合名不能为空")
		return
	}

	if !memory.Ready() {
		if err := memory.EnsureReady(); err != nil {
			writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("记忆库请求[ERROR] -> 记忆库初始化失败: %v", err))
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := memory.InitCollection(ctx, collectionName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 集合创建失败: %v", err))
		return
	}

	info := memory.CollectionInfo(collectionName)
	logger.Info("FileManager", "集合 [%s] 创建成功, 集合名向量维度: %d",
		collectionName, getIntField(info, "name_embedding_dimension"))

	writeSuccess(w, memoryCollectionInfo{
		Name:                   collectionName,
		Dimension:              getIntField(info, "embedding_dimension"),
		Count:                  getIntField(info, "document_count"),
		Version:                getIntField(info, "version"),
		HasNameEmbedding:       getBoolField(info, "has_name_embedding"),
		NameEmbeddingDimension: getIntField(info, "name_embedding_dimension"),
		NameEmbeddingModel:     getStringField(info, "name_embedding_model"),
	})
}

// handleMemoryCollectionStats GET /memory/{name}/stats — 集合统计
func handleMemoryCollectionStats(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	info := memory.CollectionInfo(collectionName)
	if info == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 集合 '%s' 不存在", collectionName))
		return
	}

	count := getIntField(info, "document_count")
	mismatch := memory.HasSyncMismatch(collectionName)

	writeSuccess(w, memoryStatsData{
		DocumentCount: count,
		Initialized:   true,
		EntryCount:    count,
		SyncMismatch:  mismatch,
	})
}

// handleMemoryMessages POST/GET/DELETE /memory/messages — 全局类型化读写
func handleMemoryMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleMemoryAddMessage(w, r)
	case http.MethodGet:
		handleMemoryQueryMessages(w, r)
	case http.MethodDelete:
		handleMemoryDeleteMessage(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST/GET/DELETE")
	}
}

// handleMemoryAddMessage POST /memory/messages — 添加记忆（内容自动归组）
func handleMemoryAddMessage(w http.ResponseWriter, r *http.Request) {
	if !memory.Ready() {
		if err := memory.EnsureReady(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
			return
		}
	}

	var req memoryAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	mediaType := strings.TrimSpace(req.Type)
	if mediaType == "" {
		// 未指定类型：带 base64 视为图片，否则按文本
		if req.Base64 != "" {
			mediaType = memory.TypeImage
		} else {
			mediaType = memory.TypeText
		}
	}
	mediaType, err := memory.NormalizeType(mediaType)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> %v", err))
		return
	}

	ctx := context.Background()
	switch mediaType {
	case memory.TypeImage:
		if req.Base64 == "" {
			writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 图片记忆需要 base64 字段")
			return
		}
		id, err := memory.AddImage(ctx, req.Base64, req.RecognitionOrientation, req.RecognitionCustom)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 添加图片失败: %v", err))
			return
		}
		logger.Info("FileManager", "添加图片记忆成功, ID: %s, 识别取向: %s", id, req.RecognitionOrientation)
		writeSuccess(w, map[string]string{"id": id, "type": mediaType})
	case memory.TypeAudio:
		if req.Base64 == "" {
			writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 音频记忆需要 base64 字段")
			return
		}
		id, err := memory.AddAudio(ctx, req.Base64, req.Description)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 添加音频失败: %v", err))
			return
		}
		logger.Info("FileManager", "添加音频记忆成功, ID: %s", id)
		writeSuccess(w, map[string]string{"id": id, "type": mediaType})
	default:
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
		var id string
		var err error
		if len(req.Tags) > 0 {
			id, err = memory.AddTextWithTags(ctx, req.Role, req.Content, req.Tags, req.Source)
		} else {
			id, err = memory.AddText(ctx, req.Role, req.Content)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 添加消息失败: %v", err))
			return
		}
		logger.Info("FileManager", "添加文本记忆成功, ID: %s, 角色: %s, 内容长度: %d", id, req.Role, len(req.Content))
		writeSuccess(w, map[string]string{"id": id, "role": req.Role, "type": mediaType})
	}
}

// handleMemoryQueryMessages GET /memory/messages — 全局检索（只指定返回类型）
func handleMemoryQueryMessages(w http.ResponseWriter, r *http.Request) {
	if !memory.Ready() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	queryText := strings.TrimSpace(r.URL.Query().Get("query"))
	if queryText == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 查询文本不能为空")
		return
	}

	mediaType, err := memory.NormalizeType(r.URL.Query().Get("type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> %v", err))
		return
	}

	topK := 10
	if topKStr := r.URL.Query().Get("top_k"); topKStr != "" {
		if val, err := strconv.Atoi(topKStr); err == nil && val > 0 && val <= 100 {
			topK = val
		}
	}

	// force_global=1 时跳过「集合名向量」快速路由，扫描全部集合（确保不漏）
	forceGlobal := false
	if v := strings.TrimSpace(r.URL.Query().Get("force_global")); v != "" {
		forceGlobal = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	ctx := context.Background()
	messages, err := memory.Query(ctx, queryText, mediaType, topK,
		memory.QueryOptions{ForceGlobal: forceGlobal})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 查询失败: %v", err))
		return
	}

	results := make([]memoryMessageData, 0, len(messages))
	for _, msg := range messages {
		results = append(results, memoryMessageData{
			ID:         msg.ID,
			Type:       msg.Type,
			Role:       msg.Role,
			Content:    msg.Content,
			URL:        msg.URL,
			Base64:     msg.Base64,
			Collection: msg.Collection,
			Source:     msg.Source,
			Similarity: msg.Similarity,
			Timestamp:  msg.Timestamp,
		})
	}

	logger.Info("FileManager", "全局查询完成（类型 %s，%s）, 查询: %s, 结果数: %d",
		mediaType, memoryQueryModeText(forceGlobal), queryText, len(results))

	writeSuccess(w, memoryQueryData{
		Query:       queryText,
		Type:        mediaType,
		TopK:        topK,
		ForceGlobal: forceGlobal,
		Results:     results,
		TotalFound:  len(results),
	})
}

// handleMemoryDeleteMessage DELETE /memory/messages — 按 id 全局删除
func handleMemoryDeleteMessage(w http.ResponseWriter, r *http.Request) {
	if !memory.Ready() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	var req memoryDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}
	if req.ID == "" {
		req.ID = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 消息ID不能为空")
		return
	}

	ctx := context.Background()
	if err := memory.Delete(ctx, req.ID); err != nil {
		if errors.Is(err, memory.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未找到 ID 为 %s 的记忆", req.ID))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 删除消息失败: %v", err))
		return
	}

	logger.Info("FileManager", "删除记忆成功, ID: %s", req.ID)
	writeSuccess(w, map[string]string{"id": req.ID})
}

// handleMemoryEntries GET /memory/entries — 按 ID 取一条，或逐条列举
func handleMemoryEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
		return
	}

	// 按 ID 返回单条
	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		entry, err := memory.Get(id)
		if err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				writeError(w, http.StatusNotFound, fmt.Sprintf("记忆库请求[ERROR] -> 未找到 ID 为 %s 的记忆", id))
				return
			}
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 读取记忆失败: %v", err))
			return
		}
		writeSuccess(w, memoryEntryOf(entry))
		return
	}

	// 逐条列举：type 为空表示全部类型；limit=0 表示全部
	mediaType := strings.TrimSpace(r.URL.Query().Get("type"))
	if mediaType != "" {
		normalized, err := memory.NormalizeType(mediaType)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("记忆库请求[ERROR] -> %v", err))
			return
		}
		mediaType = normalized
	}
	offset := 0
	if offStr := r.URL.Query().Get("offset"); offStr != "" {
		if val, err := strconv.Atoi(offStr); err == nil && val >= 0 {
			offset = val
		}
	}
	limit := 50
	if limStr := r.URL.Query().Get("limit"); limStr != "" {
		if val, err := strconv.Atoi(limStr); err == nil && val >= 0 {
			limit = val
		}
	}

	entries, total := memory.List(mediaType, offset, limit)
	list := make([]memoryMessageData, 0, len(entries))
	for i := range entries {
		list = append(list, memoryMessageData{
			ID:         entries[i].ID,
			Type:       entries[i].Type,
			Role:       entries[i].Role,
			Content:    entries[i].Content,
			URL:        entries[i].URL,
			Base64:     entries[i].Base64,
			Collection: entries[i].Collection,
			Source:     entries[i].Source,
			Similarity: entries[i].Similarity,
			Timestamp:  entries[i].Timestamp,
		})
	}

	writeSuccess(w, map[string]interface{}{
		"entries": list,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

// memoryEntryOf 单条结果 → 端点响应结构
func memoryEntryOf(entry *memory.Entry) memoryMessageData {
	if entry == nil {
		return memoryMessageData{}
	}
	return memoryMessageData{
		ID:         entry.ID,
		Type:       entry.Type,
		Role:       entry.Role,
		Content:    entry.Content,
		URL:        entry.URL,
		Base64:     entry.Base64,
		Collection: entry.Collection,
		Source:     entry.Source,
		Similarity: entry.Similarity,
		Timestamp:  entry.Timestamp,
	}
}

// handleMemoryFuse POST /memory/fuse — 权重融合（相似文本条目合并 + 重算向量）
func handleMemoryFuse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	var req memoryFuseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> 无效的请求参数")
		return
	}
	req.Collection = strings.TrimSpace(req.Collection)
	if req.Collection == "" {
		writeError(w, http.StatusBadRequest, "记忆库请求[ERROR] -> collection 不能为空")
		return
	}

	// 融合涉及逐条嵌入与可选的模型整合，给足超时；调用方断开连接时同步取消
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	result, err := memory.Fuse(ctx, req.Collection, req.IDs, memory.FuseOptions{
		Threshold: float32(req.Threshold),
		Strategy:  req.Strategy,
		Role:      req.Role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 权重融合失败: %v", err))
		return
	}

	logger.Info("FileManager", "权重融合完成 [%s]：扫描 %d 条，合并 %d 组，删除 %d 条，新增 %d 条",
		result.Collection, result.CandidateCount, result.GroupCount, result.RemovedCount, result.CreatedCount)
	writeSuccess(w, result)
}

// handleMemoryDocuments GET /memory/{name}/documents — 集合内文档分页（管理用途）
func handleMemoryDocuments(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 GET")
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

	entries, total := memory.Documents(collectionName, offset, limit)

	docList := make([]memoryMessageData, 0, len(entries))
	for _, entry := range entries {
		docList = append(docList, memoryMessageData{
			ID:         entry.ID,
			Type:       entry.Type,
			Role:       entry.Role,
			Content:    entry.Content,
			URL:        entry.URL,
			Base64:     entry.Base64,
			Collection: entry.Collection,
			Source:     entry.Source,
			Timestamp:  entry.Timestamp,
		})
	}

	writeSuccess(w, map[string]interface{}{
		"documents": docList,
		"total":     total,
		"offset":    offset,
		"limit":     limit,
	})
}

// handleMemoryRebuild POST /memory/{name}/rebuild — 重建集合内容向量
func handleMemoryRebuild(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !memory.Ready() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	ctx := context.Background()
	if err := memory.Rebuild(ctx, collectionName, "", nil); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 重建条目失败: %v", err))
		return
	}

	count := memory.DocCount(collectionName)
	logger.Info("FileManager", "集合 [%s] rebuild 完成, 剩余 %d 条文档", collectionName, count)

	writeSuccess(w, map[string]interface{}{
		"rebuilt": count,
		"message": fmt.Sprintf("集合 [%s] 重建完成, 剩余 %d 条文档", collectionName, count),
	})
}

// handleMemoryReloadCollection POST /memory/{name}/reload — 外部改写分片文件后刷新内存缓存（不重算向量）
func handleMemoryReloadCollection(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}
	if !memory.Ready() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}
	if err := memory.Reload(collectionName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 重新加载分组失败: %v", err))
		return
	}
	writeSuccess(w, map[string]string{
		"message": fmt.Sprintf("分组 [%s] 已从磁盘重新加载", collectionName),
	})
}

// handleMemoryClearCollection POST /memory/{name}/clear — 清空集合
func handleMemoryClearCollection(w http.ResponseWriter, r *http.Request, collectionName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "记忆库请求[ERROR] -> 不允许的请求方法，仅支持 POST")
		return
	}

	if !memory.Ready() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	if err := memory.ClearCollection(collectionName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 清空集合失败: %v", err))
		return
	}

	logger.Info("FileManager", "集合 [%s] 清空成功", collectionName)
	writeSuccess(w, map[string]string{
		"message": fmt.Sprintf("集合 [%s] 已清空", collectionName),
	})
}

// =============================================================================
// 辅助函数
// =============================================================================

// memoryQueryModeText 把检索范围选项转成日志里的可读文案
func memoryQueryModeText(forceGlobal bool) string {
	if forceGlobal {
		return "全局检索"
	}
	return "快速检索"
}

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

// getBoolField 从 map[string]interface{} 安全获取布尔字段
func getBoolField(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
