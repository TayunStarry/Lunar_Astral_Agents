package kokoro_tts

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// TTSHandler 语音合成接口
func TTSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TTSResponse{Success: false, Error: "无效的请求格式"})
		return
	}

	engine := GetEngine()
	if engine == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TTSResponse{Success: false, Error: "TTS 引擎未初始化"})
		return
	}

	samples, phonemes, err := engine.Synthesize(req.Text, req.Voice, req.Speed, req.Lang, req.Mix)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TTSResponse{Success: false, Error: err.Error()})
		return
	}

	wav := EncodePCMToWAV(samples, SampleRate)
	audioBase64 := base64.StdEncoding.EncodeToString(wav)

	voice := req.Voice
	if voice == "" {
		voice = defaultVoiceName
	}
	if len(req.Mix) > 0 {
		// 混合音色的显示名（按归一化比例回显）：zf_001×38%+zm_031×62%
		var total float64
		for _, m := range req.Mix {
			w := m.Weight
			if w < 0 {
				w = 0
			}
			total += w
		}
		var parts []string
		if total <= 0 {
			// 引擎等权混合
			for _, m := range req.Mix {
				parts = append(parts, fmt.Sprintf("%s×%.0f%%", m.Voice, 100.0/float64(len(req.Mix))))
			}
		} else {
			for _, m := range req.Mix {
				pct := 100.0 * m.Weight / total
				parts = append(parts, fmt.Sprintf("%s×%.0f%%", m.Voice, pct))
			}
		}
		voice = "mix:" + strings.Join(parts, "+")
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TTSResponse{
		Success:    true,
		Audio:      audioBase64,
		Phonemes:   phonemes,
		Voice:      voice,
		SampleRate: SampleRate,
	})
}

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

// HealthHandler 健康检查接口
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	engine := GetEngine()
	if engine == nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "not_ready", "service": "kokoro-tts"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"service": "kokoro-tts",
		"voices":  len(engine.voiceOrder),
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
