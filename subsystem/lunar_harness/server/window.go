package server

import (
	"encoding/json"
	"net/http"

	"LunarSubsystem/LunarHarness/engine/browser"
)

// WindowFocusHandler 置前指定桌面窗口（POST /window/focus）。
// 取代原先「检测/启动月华进程」的跨进程端点：进程已合一，改为激活对应桌面窗口。
func WindowFocusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req WindowFocusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WindowFocusResponse{Success: false, Message: "请求体解析失败"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, WindowFocusResponse{Success: false, Message: "缺少 id 字段"})
		return
	}

	if err := browser.FocusDesktopWindow(req.ID); err != nil {
		writeJSON(w, http.StatusOK, WindowFocusResponse{Success: false, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, WindowFocusResponse{Success: true, Message: "窗口已置前"})
}
