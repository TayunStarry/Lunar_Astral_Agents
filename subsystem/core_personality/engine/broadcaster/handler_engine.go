package broadcaster

import (
	"encoding/json"
	"net/http"

	KokoroTTS "CorePersonality/engine/kokoro_tts"
	OmniVoiceTTS "CorePersonality/engine/omnivoice_tts"
)

// ==== 引擎专属能力端点 ====
//
// 设计语言 · 专属能力归属：参数矩阵内公共参数走 /broadcaster/synthesize 统一入口；
// 仅存在于单一引擎的能力（音色表/读音词典/语言表）在 /broadcaster/<engine>/ 名下
// 声明专属端点。专属端点调用前自动完成槽位对齐（Activate）——「被本次调用的引擎
// 是在位的唯一引擎」这一槽位语义对专属端点同样成立；响应体保持各引擎既有形状。

// KokoroVoicesHandler 月华 /broadcaster/kokoro/voices 音色列表：
// 激活 Kokoro 槽位后输出内置音色（含语言归类）
func KokoroVoicesHandler(w http.ResponseWriter, r *http.Request) {
	withEngineSlot(w, r, EngineKokoro, KokoroTTS.VoicesHandler)
}

// KokoroDictHandler 月华 /broadcaster/kokoro/dict 读音词典管理：
// GET 查询 / POST 添加更新 / DELETE 删除（持久化到模型目录）
func KokoroDictHandler(w http.ResponseWriter, r *http.Request) {
	withEngineSlot(w, r, EngineKokoro, KokoroTTS.DictHandler)
}

// KokoroDictGuessHandler 月华 /broadcaster/kokoro/dict/guess 读音查询：
// 返回某词当前将使用的读音（用户词典优先）
func KokoroDictGuessHandler(w http.ResponseWriter, r *http.Request) {
	withEngineSlot(w, r, EngineKokoro, KokoroTTS.GuessDictHandler)
}

// OmniVoiceLanguagesHandler 月华 /broadcaster/omnivoice/languages 语言列表：
// 激活 OmniVoice 槽位后输出引擎支持的语言（id/name）
func OmniVoiceLanguagesHandler(w http.ResponseWriter, r *http.Request) {
	withEngineSlot(w, r, EngineOmniVoice, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		langs := OmniVoiceTTS.Languages()
		writeJSON(w, http.StatusOK, map[string]any{
			"success":   true,
			"languages": langs,
			"count":     len(langs),
		})
	})
}

// withEngineSlot 引擎专属能力端点共用前置：槽位对齐到指定引擎后委托引擎侧处理器；
// 激活失败（模型缺失/初始化异常）返回 JSON 错误
func withEngineSlot(w http.ResponseWriter, r *http.Request, engine string, inner http.HandlerFunc) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := Activate(engine); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": engine + " 引擎加载失败: " + err.Error()})
		return
	}
	inner(w, r)
}
