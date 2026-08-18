package server

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"net/http"
)

// writeJSON 写入 JSON 响应，设置指定状态码
func writeJSON(w http.ResponseWriter, statusCode int, resp any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// writeError 写入错误响应（success: false）
func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

// writeSuccess 写入成功响应（success: true, data: ...）
func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// =============================================================================
// 知识库操作端点 — SQL 关系型数据库（SQLite）
// =============================================================================

// KnowledgeHandler 处理知识库批量操作请求
// 路由：POST /knowledge/
func KnowledgeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "知识库请求[ERROR] -> 不允许的请求方法")
		return
	}

	var req module.KnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("知识库请求[ERROR] -> 解析请求失败: %v", err))
		return
	}

	result := module.ExecuteKnowledgeRequest(req)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("知识库请求[ERROR] -> 编码响应失败: %v", err), http.StatusInternalServerError)
		return
	}

	LoggerGeneral.Info("FileManager", "知识库批量操作成功，执行 %d 个操作，耗时 %dms", result.Operations, result.TotalTime)
}
