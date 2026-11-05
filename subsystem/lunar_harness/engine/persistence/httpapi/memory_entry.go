package httpapi

// memory_entry.go 条目视图与融合

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

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
