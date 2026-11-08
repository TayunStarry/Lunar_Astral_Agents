package server

// library_creation.go 资源库的创作提交入口：受理「资源句柄 + 自然语言描述」，
// 交给创作链后台规划执行，前端通过状态查询与 WebSocket 推送获取进度与结果。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// libraryCreateHandler 受理创作任务：assigned 需要描述，random 从随机目标池自主选题。
func libraryCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryCreationResponse{OK: false, Error: "只支持 POST"})
		return
	}
	if LibraryCreationSubmitFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, LibraryCreationResponse{OK: false, Error: "创作链尚未就绪"})
		return
	}
	var req LibraryCreationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryCreationResponse{OK: false, Error: "请求体解析失败: " + err.Error()})
		return
	}
	task, err := LibraryCreationSubmitFunc(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryCreationResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LibraryCreationResponse{OK: true, Task: &task})
}

// libraryCreateStatusHandler 查询单个任务的状态与结果。
func libraryCreateStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryCreationResponse{OK: false, Error: "只支持 GET"})
		return
	}
	if LibraryCreationStatusFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, LibraryCreationResponse{OK: false, Error: "创作链尚未就绪"})
		return
	}
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, LibraryCreationResponse{OK: false, Error: "缺少 task_id"})
		return
	}
	task, ok := LibraryCreationStatusFunc(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, LibraryCreationResponse{OK: false, Error: "任务不存在或已过期: " + taskID})
		return
	}
	writeJSON(w, http.StatusOK, LibraryCreationResponse{OK: true, Task: &task})
}

// libraryCreateListHandler 返回最近受理的创作任务，便于前端打开面板时恢复。
func libraryCreateListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryCreationListResponse{OK: false, Error: "只支持 GET"})
		return
	}
	if LibraryCreationListFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, LibraryCreationListResponse{OK: false, Error: "创作链尚未就绪"})
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, LibraryCreationListResponse{OK: false, Error: "limit 必须是正整数"})
			return
		}
		limit = parsed
	}
	writeJSON(w, http.StatusOK, LibraryCreationListResponse{OK: true, Tasks: LibraryCreationListFunc(limit)})
}
