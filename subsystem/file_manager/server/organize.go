package server

import (
	"LunarSubsystem/file_manager/module"
	"encoding/json"
	"net/http"
)

// OrganizeHandler 处理批量文件整理请求
func OrganizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Organize请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	var req module.OrganizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.OrganizeResponse{
			Success: false,
			Error:   "无效的请求体: " + err.Error(),
		})
		return
	}

	if req.BasePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.OrganizeResponse{
			Success: false,
			Error:   "base_path 不能为空",
		})
		return
	}

	if len(req.Operations) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.OrganizeResponse{
			Success: false,
			Error:   "operations 不能为空",
		})
		return
	}

	response := module.ExecuteOrganize(req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
