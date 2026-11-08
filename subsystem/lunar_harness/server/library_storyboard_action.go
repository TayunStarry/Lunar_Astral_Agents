package server

// library_storyboard_action.go 演绎任务控制入口：审查确认、暂停、继续、终止。

import (
	"encoding/json"
	"net/http"
)

// libraryStoryboardConfirmHandler 接收用户审查后的完整分镜稿并开始绘制。
func libraryStoryboardConfirmHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeStoryboardAction(w, r, LibraryStoryboardConfirmFunc != nil)
	if !ok {
		return
	}
	task, err := LibraryStoryboardConfirmFunc(req.TaskID, req.Title, req.Shots)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardResponse{OK: true, Task: &task})
}

// libraryStoryboardPauseHandler 暂停正在执行的任务。
func libraryStoryboardPauseHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeStoryboardAction(w, r, LibraryStoryboardPauseFunc != nil)
	if !ok {
		return
	}
	task, err := LibraryStoryboardPauseFunc(req.TaskID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardResponse{OK: true, Task: &task})
}

// libraryStoryboardResumeHandler 继续一个已暂停的任务。
func libraryStoryboardResumeHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeStoryboardAction(w, r, LibraryStoryboardResumeFunc != nil)
	if !ok {
		return
	}
	task, err := LibraryStoryboardResumeFunc(req.TaskID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardResponse{OK: true, Task: &task})
}

// libraryStoryboardCancelHandler 终止任务，已产出镜头保留。
func libraryStoryboardCancelHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeStoryboardAction(w, r, LibraryStoryboardCancelFunc != nil)
	if !ok {
		return
	}
	task, err := LibraryStoryboardCancelFunc(req.TaskID, req.Reason)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LibraryStoryboardResponse{OK: true, Task: &task})
}

// decodeStoryboardAction 解析控制请求；ready 为 false 时回 503。
func decodeStoryboardAction(w http.ResponseWriter, r *http.Request, ready bool) (LibraryStoryboardActionRequest, bool) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, LibraryStoryboardResponse{OK: false, Error: "只支持 POST"})
		return LibraryStoryboardActionRequest{}, false
	}
	if !ready {
		writeJSON(w, http.StatusServiceUnavailable, LibraryStoryboardResponse{OK: false, Error: "演绎链尚未就绪"})
		return LibraryStoryboardActionRequest{}, false
	}
	var req LibraryStoryboardActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: "请求体解析失败: " + err.Error()})
		return LibraryStoryboardActionRequest{}, false
	}
	if req.TaskID == "" {
		writeJSON(w, http.StatusBadRequest, LibraryStoryboardResponse{OK: false, Error: "缺少 task_id"})
		return LibraryStoryboardActionRequest{}, false
	}
	return req, true
}
