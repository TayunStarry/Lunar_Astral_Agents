package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"logger"
	"math/rand"
	"net/http"
	"sd_lunar/engine"
	"strings"
	"time"
)

type HTTPServer struct {
	addr   string
	engine *engine.Engine
	mux    *http.ServeMux
}

func NewHTTPServer(addr string) *HTTPServer {
	s := &HTTPServer{
		addr:   addr,
		engine: engine.GetEngine(),
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *HTTPServer) registerRoutes() {
	s.mux.HandleFunc("/api/v1/txt2img", s.handleTxt2Img)
	s.mux.HandleFunc("/api/v1/img2img", s.handleImg2Img)
	s.mux.HandleFunc("/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("/api/v1/ping", s.handlePing)
}

func (s *HTTPServer) Start() error {
	logger.Info("SD-LUNAR", "HTTP服务启动于 %s", s.addr)
	handler := RecoveryMiddleware(CORSMiddleware(s.mux))
	server := &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *HTTPServer) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok", "service": "sd_lunar"})
}

func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	ready := s.engine != nil && s.engine.IsReady()
	sysInfo := ""
	if ready {
		sysInfo = "SD engine ready"
	}
	writeJSON(w, StatusResponse{
		Success: true,
		Ready:   ready,
		Info:    sysInfo,
	})
}

func (s *HTTPServer) handleTxt2Img(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, GenerateResponse{Success: false, Message: "仅支持POST请求"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("SD-LUNAR", "读取请求体失败: %v", err)
		writeJSON(w, GenerateResponse{Success: false, Message: "读取请求体失败"})
		return
	}

	var req Txt2ImgRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("SD-LUNAR", "解析请求参数失败: %v", err)
		writeJSON(w, GenerateResponse{Success: false, Message: "解析请求参数失败: " + err.Error()})
		return
	}

	if req.Prompt == "" {
		writeJSON(w, GenerateResponse{Success: false, Message: "提示词不能为空"})
		return
	}

	cfg := normalizeTxt2ImgConfig(req)

	logger.Info("SD-LUNAR", "文生图请求: prompt=%s, size=%dx%d, steps=%d, sampler=%s, cfg=%.1f",
		truncateString(cfg.Prompt, 80), cfg.Width, cfg.Height, cfg.Steps, cfg.Sampler, cfg.CFGScale)

	img, err := s.engine.GenerateTextToImage(cfg)
	if err != nil {
		logger.Error("SD-LUNAR", "文生图失败: %v", err)
		writeJSON(w, GenerateResponse{Success: false, Message: "生成失败: " + err.Error()})
		return
	}

	base64Data := base64.StdEncoding.EncodeToString(img.Data)
	logger.Info("SD-LUNAR", "文生图完成: %dx%d, %.1fKB", img.Width, img.Height, float64(len(img.Data))/1024)

	writeJSON(w, GenerateResponse{
		Success: true,
		Data:    base64Data,
		Width:   int(img.Width),
		Height:  int(img.Height),
		Seed:    img.Seed,
	})
}

func (s *HTTPServer) handleImg2Img(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, GenerateResponse{Success: false, Message: "仅支持POST请求"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("SD-LUNAR", "读取请求体失败: %v", err)
		writeJSON(w, GenerateResponse{Success: false, Message: "读取请求体失败"})
		return
	}

	var req Img2ImgRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error("SD-LUNAR", "解析请求参数失败: %v", err)
		writeJSON(w, GenerateResponse{Success: false, Message: "解析请求参数失败: " + err.Error()})
		return
	}

	if req.Prompt == "" {
		writeJSON(w, GenerateResponse{Success: false, Message: "提示词不能为空"})
		return
	}
	if req.ImageBase64 == "" {
		writeJSON(w, GenerateResponse{Success: false, Message: "参考图片数据不能为空"})
		return
	}

	refData, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil {
		writeJSON(w, GenerateResponse{Success: false, Message: "参考图片Base64解码失败: " + err.Error()})
		return
	}

	refWidth, refHeight := detectImageDimensions(refData)
	if refWidth == 0 || refHeight == 0 {
		writeJSON(w, GenerateResponse{Success: false, Message: "无法识别参考图片尺寸"})
		return
	}

	cfg := normalizeImg2ImgConfig(req)

	logger.Info("SD-LUNAR", "图生图请求: prompt=%s, ref=%dx%d, size=%dx%d, steps=%d, strength=%.2f",
		truncateString(cfg.Prompt, 80), refWidth, refHeight, cfg.Width, cfg.Height, cfg.Steps, cfg.Strength)

	img, err := s.engine.GenerateImageToImage(cfg, refData, uint32(refWidth), uint32(refHeight), 3)
	if err != nil {
		logger.Error("SD-LUNAR", "图生图失败: %v", err)
		writeJSON(w, GenerateResponse{Success: false, Message: "生成失败: " + err.Error()})
		return
	}

	base64Data := base64.StdEncoding.EncodeToString(img.Data)
	logger.Info("SD-LUNAR", "图生图完成: %dx%d, %.1fKB", img.Width, img.Height, float64(len(img.Data))/1024)

	writeJSON(w, GenerateResponse{
		Success: true,
		Data:    base64Data,
		Width:   int(img.Width),
		Height:  int(img.Height),
		Seed:    img.Seed,
	})
}

func normalizeTxt2ImgConfig(req Txt2ImgRequest) engine.GenerationConfig {
	cfg := engine.GenerationConfig{
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Width:          clampInt(req.Width, 64, 2048, 512),
		Height:         clampInt(req.Height, 64, 2048, 512),
		Steps:          clampInt(req.Steps, 1, 150, 20),
		Sampler:        defaultIfEmpty(req.Sampler, "euler_a"),
		Scheduler:      defaultIfEmpty(req.Scheduler, "discrete"),
		CFGScale:       clampFloat(float32(req.CFGScale), 1.0, 30.0, 7.0),
		Seed:           req.Seed,
		ClipSkip:       clampInt(req.ClipSkip, -1, 12, -1),
		BatchCount:     1,
	}
	if cfg.Seed == 0 {
		cfg.Seed = rand.Int63()
	}
	return cfg
}

func normalizeImg2ImgConfig(req Img2ImgRequest) engine.GenerationConfig {
	cfg := engine.GenerationConfig{
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Width:          clampInt(req.Width, 64, 2048, 512),
		Height:         clampInt(req.Height, 64, 2048, 512),
		Steps:          clampInt(req.Steps, 1, 150, 20),
		Sampler:        defaultIfEmpty(req.Sampler, "euler_a"),
		Scheduler:      defaultIfEmpty(req.Scheduler, "discrete"),
		CFGScale:       clampFloat(float32(req.CFGScale), 1.0, 30.0, 7.0),
		Seed:           req.Seed,
		ClipSkip:       clampInt(req.ClipSkip, -1, 12, -1),
		Strength:       clampFloat(float32(req.Strength), 0.01, 1.0, 0.75),
		BatchCount:     1,
	}
	if cfg.Seed == 0 {
		cfg.Seed = rand.Int63()
	}
	return cfg
}

func detectImageDimensions(data []byte) (int, int) {
	if len(data) < 24 {
		return 0, 0
	}

	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		if len(data) >= 24 {
			w := int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
			h := int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
			return w, h
		}
	}

	if data[0] == 0xFF && data[1] == 0xD8 {
		return detectJPEGDimensions(data)
	}

	if data[0] == 'B' && data[1] == 'M' {
		if len(data) >= 26 {
			w := int(data[18]) | int(data[19])<<8
			h := int(data[22]) | int(data[23])<<8
			return w, h
		}
	}

	if data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' {
		if len(data) >= 26 {
			w := int(data[20]) | int(data[21])<<8
			h := int(data[22]) | int(data[23])<<8
			return w + 1, h + 1
		}
	}

	return 0, 0
}

func detectJPEGDimensions(data []byte) (int, int) {
	i := 2
	for i < len(data)-8 {
		if data[i] != 0xFF {
			return 0, 0
		}
		marker := data[i+1]
		if marker == 0xD8 || marker == 0xD9 || marker == 0x01 {
			i += 2
			continue
		}
		if marker >= 0xC0 && marker <= 0xC3 {
			h := int(data[i+5])<<8 | int(data[i+6])
			w := int(data[i+7])<<8 | int(data[i+8])
			return w, h
		}
		if i+2 >= len(data) {
			break
		}
		length := int(data[i+2])<<8 | int(data[i+3])
		i += 2 + length
	}
	return 0, 0
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("SD-LUNAR", "JSON编码失败: %v", err)
	}
}

func clampInt(v, min, max, def int) int {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampFloat(v, min, max, def float32) float32 {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func defaultIfEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("SD-LUNAR", "panic恢复: %v", rec)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"success":false,"message":"内部错误: %v"}`, rec)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
