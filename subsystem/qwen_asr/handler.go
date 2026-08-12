package main

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type AsrResponse struct {
	Status      string  `json:"status"`
	Confidence  float64 `json:"confidence"`
	Text        string  `json:"text"`
	Error       string  `json:"error,omitempty"`
	AudioFormat string  `json:"audio_format"`
}

type AsrHandler struct {
	asr       *QwenASR
	uploadDir string
}

func NewAsrHandler(asr *QwenASR, uploadDir string) *AsrHandler {
	return &AsrHandler{
		asr:       asr,
		uploadDir: uploadDir,
	}
}

func (h *AsrHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.URL.Path {
	case "/health":
		h.handleHealth(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, AsrResponse{
			Status: "error",
			Error:  "Only POST method is allowed",
		})
		return
	}

	switch r.URL.Path {
	case "/asr", "/asr/":
		h.handleAsr(w, r)
	default:
		h.sendJSON(w, http.StatusNotFound, AsrResponse{
			Status: "error",
			Error:  "Not found",
		})
	}
}

func (h *AsrHandler) handleAsr(w http.ResponseWriter, r *http.Request) {
	LoggerGeneral.Info("ASREngine", "Received ASR request")

	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		LoggerGeneral.Error("ASREngine", "Failed to parse form: %v", err)
		h.sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status: "error",
			Error:  "Failed to parse form data",
		})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		LoggerGeneral.Error("ASREngine", "Failed to get audio file: %v", err)
		h.sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status: "error",
			Error:  "Missing audio file (use form field 'audio')",
		})
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		LoggerGeneral.Error("ASREngine", "Failed to read audio data: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Failed to read audio data",
		})
		return
	}

	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		LoggerGeneral.Error("ASREngine", "Failed to create upload dir: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Server error",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	tmpPath := filepath.Join(h.uploadDir, header.Filename)

	var wavData []byte

	if isValidWav(audioData) {
		wavData = audioData
	} else {
		LoggerGeneral.Info("ASREngine", "Non-WAV format detected (filename: %s), converting to WAV", header.Filename)
		srcExt := ext
		if srcExt != ".webm" && srcExt != ".ogg" && srcExt != ".mp4" && srcExt != ".m4a" && srcExt != ".weba" {
			srcExt = ".webm"
		}
		var err error
		wavData, err = convertToWav(audioData, srcExt)
		if err != nil {
			LoggerGeneral.Error("ASREngine", "Failed to convert audio: %v", err)
			h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
				Status: "error",
				Error:  fmt.Sprintf("Audio conversion failed: %v", err),
			})
			return
		}
		tmpPath = filepath.Join(h.uploadDir, "converted_"+header.Filename+".wav")
	}

	if err := os.WriteFile(tmpPath, wavData, 0644); err != nil {
		LoggerGeneral.Error("ASREngine", "Failed to save audio file: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Failed to save audio file",
		})
		return
	}
	defer os.Remove(tmpPath)

	LoggerGeneral.Info("ASREngine", "Processing audio file: %s (%d bytes, format=%s)", header.Filename, len(wavData), ext)

	text, err := h.asr.TranscribeWavFile(tmpPath)
	if err != nil {
		LoggerGeneral.Error("ASREngine", "ASR transcription failed: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status:      "error",
			Error:       fmt.Sprintf("Transcription failed: %v", err),
			AudioFormat: ext,
		})
		return
	}

	confidence := estimateConfidence(text)
	LoggerGeneral.Info("ASREngine", "Transcription complete: %s (confidence: %.2f)", text, confidence)

	h.sendJSON(w, http.StatusOK, AsrResponse{
		Status:      "success",
		Confidence:  confidence,
		Text:        text,
		AudioFormat: "wav",
	})
}

func isValidWav(data []byte) bool {
	if len(data) < 44 {
		return false
	}
	return data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'A' && data[10] == 'V' && data[11] == 'E'
}

func convertToWav(inputData []byte, srcExt string) ([]byte, error) {
	tmpInput := filepath.Join(os.TempDir(), fmt.Sprintf("asr_input_%d%s", time.Now().UnixNano(), srcExt))
	tmpOutput := filepath.Join(os.TempDir(), fmt.Sprintf("asr_output_%d.wav", time.Now().UnixNano()))

	if err := os.WriteFile(tmpInput, inputData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp input: %w", err)
	}
	defer os.Remove(tmpInput)
	defer os.Remove(tmpOutput)

	ffmpegPath := findFfmpeg()
	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpeg not found in PATH, required for audio conversion")
	}

	cmd := exec.Command(ffmpegPath,
		"-i", tmpInput,
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		tmpOutput,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg conversion failed: %v, output: %s", err, string(output))
	}

	wavData, err := os.ReadFile(tmpOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted WAV: %w", err)
	}

	return wavData, nil
}

func findFfmpeg() string {
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}
	commonPaths := []string{
		`C:\ffmpeg\bin\ffmpeg.exe`,
		`C:\Program Files\ffmpeg\bin\ffmpeg.exe`,
	}
	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (h *AsrHandler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h.sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"service": "qwen-asr-server",
		"port":    35768,
	})
}

func (h *AsrHandler) sendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func estimateConfidence(text string) float64 {
	if text == "" {
		return 0.0
	}

	length := len([]rune(text))

	if length <= 5 {
		return 0.6
	} else if length <= 20 {
		return 0.75
	} else if length <= 50 {
		return 0.85
	}

	return 0.9
}
