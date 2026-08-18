package server

import (
	"LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"net/http"
	"path/filepath"
)

// HashRenameHandler 对指定目录（相对 LocalDir）下的全部文件执行哈希命名
// 请求体: {"path": "目标目录相对路径，空表示根目录"}
func HashRenameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	var req module.HashRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, module.HashRenameResponse{
			Success: false,
			Error:   "请求体解析失败: " + err.Error(),
		})
		return
	}

	response, err := module.HashRenameDir(filepath.Clean(*GeneralConfig.LocalDir), req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response)
		return
	}

	LoggerGeneral.SubInfo("FileManager", "HashRename", "哈希命名完成: %s, 重命名 %d 个文件", req.Path, response.Renamed)
	writeJSON(w, http.StatusOK, response)
}
