package crystal

import (
	"net/http"

	ttsrouter "CorePersonality/engine/tts_router"
)

// OmniVoiceHandler 包装 OmniVoice 处理器：请求到达时确保引擎已就绪（惰性加载与
// 闲置卸载统一由 engine/tts_router 托管），失败返回 JSON 错误
func OmniVoiceHandler(inner http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ttsrouter.EnsureOmniVoice(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "OmniVoice 引擎初始化失败: " + err.Error(),
			})
			return
		}
		inner(w, r)
	}
}
