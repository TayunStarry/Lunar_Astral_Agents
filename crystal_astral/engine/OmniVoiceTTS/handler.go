package OmniVoiceTTS

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
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
	if req.Text == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TTSResponse{Success: false, Error: "文本内容不能为空"})
		return
	}

	engine := GetEngine()
	if engine == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TTSResponse{Success: false, Error: "引擎未初始化"})
		return
	}

	samples, err := engine.Synthesize(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TTSResponse{Success: false, Error: err.Error()})
		return
	}

	wav := EncodePCMToWAV(samples, SampleRate)
	voice := req.Instruct
	if voice == "" && req.RefAudio != "" {
		voice = "clone"
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TTSResponse{
		Success:    true,
		Audio:      base64.StdEncoding.EncodeToString(wav),
		Voice:      voice,
		SampleRate: SampleRate,
	})
}

// LanguagesHandler 支持的语言列表
func LanguagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	langs := Languages()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"languages": langs,
		"count":     len(langs),
	})
}

// HealthHandler 引擎健康检查
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "ok",
		Service:   "omnivoice-tts",
		Loaded:    IsLoaded(),
		Languages: len(Languages()),
	})
}
