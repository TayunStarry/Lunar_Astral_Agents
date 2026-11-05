package httpapi

// memory_messages.go 检索写入删除端点

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
)

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

	logger.Info("FileManager", "全局查询完成（类型 %s，%s）, 查询: %s, 结果数: %d", mediaType, memoryQueryModeText(forceGlobal), queryText, len(results))

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

	writeSuccess(w, map[string]any{
		"entries": list,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

// memoryEntryOf 单条结果 → 端点响应结构
