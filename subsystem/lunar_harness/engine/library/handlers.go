package library

// handlers.go 资源库 HTTP 端点：清单 / 状态 / 详情 / 重命名 / 移动 / 删除。

import (
	"encoding/json"
	"net/http"
	"strings"
)

// TreeHandler GET /library/tree — 递归返回 4 个存储根的完整结构。
func TreeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, ListAll())
}

// EditsHandler GET /library/edits — 返回全部文件的最近操作与视觉状态。
func EditsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, Snapshot())
}

// MetaHandler GET /library/meta?path=documents/x.png — 单文件详情字段。
func MetaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	meta, err := MetaOf(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MetaResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, MetaResponse{OK: true, Meta: meta})
}

// RenameHandler POST /library/rename — 同目录内重命名。
func RenameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}
	req, ok := decodeOp(w, r)
	if !ok {
		return
	}
	target, err := Rename(req.Path, req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, OpResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, OpResponse{OK: true, Path: target})
}

// MoveHandler POST /library/move — 移动到指定目录。
func MoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}
	req, ok := decodeOp(w, r)
	if !ok {
		return
	}
	target, err := Move(req.Path, req.Target)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, OpResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, OpResponse{OK: true, Path: target})
}

// DeleteHandler POST /library/delete — 删除文件。
func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}
	req, ok := decodeOp(w, r)
	if !ok {
		return
	}
	target, err := Delete(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, OpResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, OpResponse{OK: true, Path: target})
}

// decodeOp 解析管理操作请求体。
func decodeOp(w http.ResponseWriter, r *http.Request) (OpRequest, bool) {
	var req OpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, OpResponse{OK: false, Error: "无效的请求体"})
		return OpRequest{}, false
	}
	if strings.TrimSpace(req.Path) == "" {
		writeJSON(w, http.StatusBadRequest, OpResponse{OK: false, Error: "缺少 path"})
		return OpRequest{}, false
	}
	return req, true
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
