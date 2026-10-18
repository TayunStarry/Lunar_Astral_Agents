package server

import (
	"LunarSubsystem/FileManager/module"
	"encoding/json"
	"net/http"
	"strings"
)

// ResourceAdmitHandler 文档资源入库/刷新：按原文件名登记并返回句柄
//
// 与通用资源入库的区别：同一 documents/ 路径始终复用同一句柄，
// 内容变化时只刷新哈希、体积与段落表，避免每次保存都新增 @D 序号。
func ResourceAdmitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "ResourceAdmit请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	var req resourceAdmitDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "ResourceAdmit请求[ERROR] -> 参数解析失败", http.StatusBadRequest)
		return
	}
	if req.Path == "" || req.Name == "" {
		http.Error(w, "ResourceAdmit请求[ERROR] -> path 与 name 不能为空", http.StatusBadRequest)
		return
	}
	res, err := module.ResourceAdmitDocument(req.Path, req.Name)
	if err != nil {
		http.Error(w, "ResourceAdmit请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "resource": res})
}

// ResourceResolveHandler 按句柄解析资源目录条目
//
// GET /file/resource/resolve?handle=@D.006
// 供前端按句柄定位文档库文件；不读取正文。
func ResourceResolveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "ResourceResolve请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}
	handle := strings.TrimSpace(r.URL.Query().Get("handle"))
	if handle == "" {
		http.Error(w, "ResourceResolve请求[ERROR] -> handle 不能为空", http.StatusBadRequest)
		return
	}
	res, err := module.ResourceResolve(handle)
	if err != nil {
		http.Error(w, "ResourceResolve请求[ERROR] -> "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "resource": res})
}
