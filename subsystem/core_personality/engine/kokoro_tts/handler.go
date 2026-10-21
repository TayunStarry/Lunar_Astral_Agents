package KokoroTTS

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ==== 引擎侧能力接口（经统一播报链路 engine/broadcaster 暴露） ====
//
// 引擎包不再持有直连 HTTP 合成端点：合成一律走 /broadcaster/synthesize 统一入口；
// 本文件仅保留引擎专属能力处理器（音色表 / 读音词典），由 broadcaster 的
// /broadcaster/kokoro/* 端点在完成槽位对齐后委托调用，响应形状保持原样。

// VoicesHandler 音色列表接口
func VoicesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	engine := GetEngine()
	if engine == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "TTS 引擎未初始化"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"voices":  engine.ListVoices(),
		"count":   len(engine.voiceOrder),
	})
}

// DictHandler 读音词典管理接口（GET 查询 / POST 添加更新 / DELETE 删除）
func DictHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if pronunciationDict == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "读音词典未初始化"})
		return
	}

	switch r.Method {
	case "GET":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"count":   len(pronunciationDict.All()),
			"entries": pronunciationDict.All(),
		})
	case "POST":
		var req DictRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "无效的请求格式"})
			return
		}
		if err := pronunciationDict.Set(req.Word, req.Pinyin); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"word":    req.Word,
			"pinyin":  pronunciationDict.All()[req.Word],
		})
	case "DELETE":
		word := r.URL.Query().Get("word")
		if err := pronunciationDict.Delete(word); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "word": word})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GuessDictHandler 读音查询接口：返回某个词当前会使用的读音（用户词典优先）
func GuessDictHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	word := strings.TrimSpace(r.URL.Query().Get("word"))
	if word == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "缺少 word 参数"})
		return
	}

	initials, finals := getInitialsFinals(word)
	pinyins := reconstructPinyin(initials, finals)
	_, inDict := pronunciationDict.Get(word)

	json.NewEncoder(w).Encode(GuessResponse{
		Success: true,
		Word:    word,
		Pinyin:  strings.Join(pinyins, " "),
		InDict:  inDict,
	})
}
