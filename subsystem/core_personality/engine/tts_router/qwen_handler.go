package ttsrouter

// ==== Qwen 专用端点（/qwen 系列） ====
// 与 /tts 统一端点共用适配执行层（synthQwen），但不按 tts.engine 配置分发——强制 Qwen 引擎。
// 供明确锁定 Qwen 音色的调用方使用（琉璃播报、蓝图 qwen_tts 节点、qwen-tts 前端工作台）；
// 需要随配置切换引擎的场景请调 /tts 统一端点。

import (
	"encoding/json"
	"net/http"

	qwentts "CorePersonality/engine/qwen_tts"
)

// QwenTTSHandler 月华 /qwen/tts 专用入口：强制 Qwen 引擎合成。
// 请求格式与统一参数样式一致（text/ref_audio 为核心字段；Qwen 不支持的
// voice/speed/mix 等音色字段自动忽略），qwen-tts 前端工作台的采样参数
// （temperature/top_k/top_p/max_tokens/repetition_penalty/threads/disable_cache）原样透传。
func QwenTTSHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := serveTTSPOST(w, r)
	if !ok {
		return
	}
	payload, status := synthQwen(req)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// QwenHealthHandler 月华 /qwen/health 专用入口：引擎健康检查。
// 与 /kokoro/health、/omnivoice/health 对齐：请求到达即确保引擎就绪（惰性加载），
// 不触碰闲置计时（touchEngine 仅真实合成路径更新，health 轮询不会阻止闲置卸载）。
func QwenHealthHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := EnsureQwen(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"status": "error", "service": "qwen-tts", "error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"service":        "qwen-tts",
		"last_used_unix": qwentts.LastUsedUnix(),
	})
}
