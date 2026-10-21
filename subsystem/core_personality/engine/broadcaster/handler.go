package broadcaster

import (
	"encoding/json"
	"net/http"
	"strings"

	"LunarSubsystem/GeneralConfig"
)

// ==== /broadcaster 核心端点族 ====
//
// 端点族总览（完整参数契约见各处理器注释与 /api-docs）：
//   POST /broadcaster/synthesize    唯一合成入口（engine 参数选引擎，缺省 tts.engine 配置）
//   GET  /broadcaster/status        槽位观测快照（无副作用，不加载引擎）
//   POST /broadcaster/activate      显式激活引擎（预加载/切换，body: {"engine":"Kokoro"}）
//   POST /broadcaster/release       显式卸载在位引擎（幂等）
//   GET  /broadcaster/capabilities  引擎能力矩阵（静态声明，不触碰引擎）
//   GET  /broadcaster/kokoro/…      Kokoro 专属能力（音色表/读音词典，见 handler_engine.go）
//   GET  /broadcaster/omnivoice/…   OmniVoice 专属能力（语言表，见 handler_engine.go）

// SynthesizeHandler 月华 /broadcaster/synthesize 统一合成入口：
// 请求体为 TTSRequest 统一参数样式，engine 参数指定本次调用的引擎，空或非法取值
// 回退 tts.engine 配置（默认 Qwen）。目标引擎未在位时自动执行槽位热切换
// （等待在位合成完毕 → 卸载 → 加载 → 合成），成功响应统一携带 engine/elapsed_ms。
func SynthesizeHandler(w http.ResponseWriter, r *http.Request) {
	if !serveBroadcastPOST(w, r) {
		return
	}
	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "无效的请求格式"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "文本内容不能为空"})
		return
	}
	payload, status := Synthesize(req)
	writeJSON(w, status, payload)
}

// StatusHandler 月华 /broadcaster/status 槽位观测入口：
// 返回当前在位引擎与状态机快照。零副作用——不加载、不卸载、不触碰闲置计时。
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	serveBroadcastGET(w, r, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "slot": Snapshot()})
	})
}

// ActivateHandler 月华 /broadcaster/activate 显式激活入口：
// 把槽位切换到指定引擎并等待加载完成（预加载消除首响延迟；已在位则幂等）。
// 响应携带激活后的槽位快照。
func ActivateHandler(w http.ResponseWriter, r *http.Request) {
	if !serveBroadcastPOST(w, r) {
		return
	}
	var req struct {
		Engine string `json:"engine"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "无效的请求格式"})
		return
	}
	engine := resolveEngine(req.Engine)
	if err := Activate(engine); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "slot": Snapshot()})
}

// ReleaseHandler 月华 /broadcaster/release 显式卸载入口：清空引擎槽位（幂等空操作）
func ReleaseHandler(w http.ResponseWriter, r *http.Request) {
	if !serveBroadcastPOST(w, r) {
		return
	}
	if err := Release(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "slot": Snapshot()})
}

// CapabilitiesHandler 月华 /broadcaster/capabilities 能力矩阵入口：
// 输出三引擎参数支持范围与专属端点声明（静态数据，不加载引擎）。
// 调用方按能力探测适配引擎差异，替代对旧引擎专属端点的硬编码依赖。
func CapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	serveBroadcastGET(w, r, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":        true,
			"default_engine": *GeneralConfig.TTSEngine,
			"engines":        engineCapabilities,
		})
	})
}

// ==== HTTP 前置辅助 ====

// serveBroadcastPOST 公共前置：CORS + 方法守卫（POST/OPTIONS）
func serveBroadcastPOST(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return false
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// serveBroadcastGET 公共前置：CORS + 方法守卫（GET/OPTIONS）+ 委托执行
func serveBroadcastGET(w http.ResponseWriter, r *http.Request, inner func(http.ResponseWriter, *http.Request)) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	inner(w, r)
}

// writeJSON 统一 JSON 响应写出
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
