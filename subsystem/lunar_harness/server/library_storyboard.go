package server

// library_storyboard.go 资源库的演绎链入口：受理「故事文本 / 资源句柄」，
// 交给演绎链后台拆分镜并逐镜绘图，前端通过状态查询与 WebSocket 推送获取进度。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// libraryStoryboardHandler 受理演绎任务。
func libraryStoryboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryStoryboardResponse{OK: false, Error: "只支持 POST"})
		return
	}
	if LibraryStoryboardSubmitFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, LibraryStoryboardResponse{OK: false, Error: "演绎链尚未就绪"})
		return
	}
	var req LibraryStoryboardRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: "请求体解析失败: " + err.Error()})
		return
	}
	task, err := LibraryStoryboardSubmitFunc(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardResponse{OK: true, Task: &task})
}

// libraryStoryboardStatusHandler 查询单个演绎任务的状态与结果。
func libraryStoryboardStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryStoryboardResponse{OK: false, Error: "只支持 GET"})
		return
	}
	if LibraryStoryboardStatusFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, LibraryStoryboardResponse{OK: false, Error: "演绎链尚未就绪"})
		return
	}
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: "缺少 task_id"})
		return
	}
	task, ok := LibraryStoryboardStatusFunc(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, LibraryStoryboardResponse{OK: false, Error: "任务不存在或已过期: " + taskID})
		return
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardResponse{OK: true, Task: &task})
}

// libraryStoryboardListHandler 返回最近受理的演绎任务。
func libraryStoryboardListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryStoryboardListResponse{OK: false, Error: "只支持 GET"})
		return
	}
	if LibraryStoryboardListFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, LibraryStoryboardListResponse{OK: false, Error: "演绎链尚未就绪"})
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, LibraryStoryboardListResponse{OK: false, Error: "limit 必须是正整数"})
			return
		}
		limit = parsed
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardListResponse{OK: true, Tasks: LibraryStoryboardListFunc(limit)})
}
