package api

import (
	"encoding/json"
	"net/http"

	"lunartick/engine"

	"logger"
)

type APIHandler struct {
	eng *engine.Engine
}

func NewAPIHandler(eng *engine.Engine) *APIHandler {
	return &APIHandler{eng: eng}
}

func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/status", h.handleStatus)
	mux.HandleFunc("/api/run", h.handleRun)
	mux.HandleFunc("/api/load", h.handleLoad)
	mux.HandleFunc("/api/inject", h.handleInject)
	mux.HandleFunc("/api/invoke", h.handleInvoke)
	mux.HandleFunc("/api/variables", h.handleVariables)
	mux.HandleFunc("/api/pointers", h.handlePointers)
	mux.HandleFunc("/api/start", h.handleStart)
	mux.HandleFunc("/api/stop", h.handleStop)
	mux.HandleFunc("/api/shutdown", h.handleShutdown)
	mux.HandleFunc("/api/health", h.handleHealth)
}

func (h *APIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	stats := h.eng.GetStats()
	vars := h.eng.GetAllVariables()
	pointers := h.eng.GetPointerNames()

	engineErrors := h.eng.GetErrors()
	var apiErrors []ErrorEntry
	for _, e := range engineErrors {
		apiErrors = append(apiErrors, ErrorEntry{
			BlockID:    e.BlockID,
			Message:    e.Message,
			TickNumber: e.TickNumber,
		})
	}

	resp := StatusResponse{
		Running:       h.eng.IsRunning(),
		Suspended:     h.eng.IsSuspended(),
		TickNumber:    stats.TickNumber,
		ReadyBlocks:   stats.ReadyBlocks,
		WaitingBlocks: stats.WaitingBlocks,
		Variables:     vars,
		Pointers:      pointers,
		Errors:        apiErrors,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *APIHandler) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	lines := []string{"@run '" + req.Path + "'"}
	for _, arg := range req.Args {
		lines[0] += " '" + arg + "'"
	}

	h.eng.Inject(lines)
	logger.Info("LunarAPI", "注入运行块: %s %v", req.Path, req.Args)

	writeJSON(w, http.StatusOK, RunResponse{
		BlockID: "injected",
		Status:  "running",
	})
}

func (h *APIHandler) handleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req LoadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	switch req.Format {
	case "markdown", "md":
		h.eng.LoadMarkdown(req.Source)
	case "json":
		var jsonArray [][]string
		if err := json.Unmarshal([]byte(req.Source), &jsonArray); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON source: "+err.Error())
			return
		}
		h.eng.LoadJSON(jsonArray)
	default:
		h.eng.LoadMarkdown(req.Source)
	}

	writeJSON(w, http.StatusOK, LoadResponse{
		Status: "loaded",
	})
}

func (h *APIHandler) handleInject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req InjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	h.eng.Inject(req.Lines)
	logger.Info("LunarAPI", "注入代码块: %d 行", len(req.Lines))

	writeJSON(w, http.StatusOK, InjectResponse{
		BlockCount: 1,
		Status:     "injected",
	})
}

func (h *APIHandler) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	h.eng.Invoke(req.Pointer)
	logger.Info("LunarAPI", "调用指针: %s", req.Pointer)

	writeJSON(w, http.StatusOK, InvokeResponse{
		Status: "invoked",
	})
}

func (h *APIHandler) handleVariables(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		vars := h.eng.GetAllVariables()
		writeJSON(w, http.StatusOK, vars)

	case http.MethodPost:
		var req VariableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		h.eng.SetVariable(req.Name, req.Value)
		writeJSON(w, http.StatusOK, VariableResponse{
			Name:  req.Name,
			Value: req.Value,
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *APIHandler) handlePointers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	pointers := h.eng.GetPointerNames()
	if pointers == nil {
		pointers = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pointers": pointers,
	})
}

func (h *APIHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	h.eng.Start()
	logger.Info("LunarAPI", "引擎已启动")

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "started",
	})
}

func (h *APIHandler) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	h.eng.Stop()
	logger.Info("LunarAPI", "引擎已停止")

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "stopped",
	})
}

func (h *APIHandler) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	h.eng.Shutdown()
	logger.Info("LunarAPI", "引擎已关闭")

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "shutdown",
	})
}

func (h *APIHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"running": h.eng.IsRunning(),
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}
