package server

import (
	"LunarSubsystem/FileManager/module"
	"encoding/json"
	"net/http"
)

// MoveHandler 处理文件移动请求
// POST /file/move
// 请求体: {"sources":["a.txt","dir"],"target_dir":"dest","conflict_strategy":"ask","create_dirs":false}
func MoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Move请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	var req module.MoveItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.MoveResponse{
			Success: false,
			Error:   "无效的请求体: " + err.Error(),
		})
		return
	}

	if len(req.Sources) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.MoveResponse{
			Success: false,
			Error:   "sources 不能为空",
		})
		return
	}

	// 默认冲突策略为 ask（预检，不自动执行）
	if req.ConflictStrategy == "" {
		req.ConflictStrategy = "ask"
	}

	response := module.ExecuteMove(req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
