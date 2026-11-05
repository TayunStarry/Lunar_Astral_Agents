package httpapi

// memory_collections.go 集合维护端点

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"context"
	"fmt"
	"net/http"
	"strconv"
)

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

	writeSuccess(w, map[string]any{
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

	writeSuccess(w, map[string]any{
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

// handleMemoryDeleteCollection DELETE /memory/{name} — 删除集合
func handleMemoryDeleteCollection(w http.ResponseWriter, _ *http.Request, collectionName string) {
	if !memory.Ready() {
		writeError(w, http.StatusServiceUnavailable, "记忆库请求[ERROR] -> 记忆库未初始化")
		return
	}

	if err := memory.DeleteCollection(collectionName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("记忆库请求[ERROR] -> 删除集合失败: %v", err))
		return
	}

	logger.Info("FileManager", "集合 [%s] 删除成功", collectionName)
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
