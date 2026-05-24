package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	logger    *log.Logger
}

func NewAsrHandler(asr *QwenASR, uploadDir string) *AsrHandler {
	return &AsrHandler{
		asr:       asr,
		uploadDir: uploadDir,
		logger:    log.New(os.Stdout, "[ASR-HTTP] ", log.LstdFlags),
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
	h.logger.Println("Received ASR request")

	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		h.logger.Printf("Failed to parse form: %v", err)
		h.sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status: "error",
			Error:  "Failed to parse form data",
		})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		h.logger.Printf("Failed to get audio file: %v", err)
		h.sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status: "error",
			Error:  "Missing audio file (use form field 'audio')",
		})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".wav") {
		h.logger.Printf("Unsupported file format: %s", header.Filename)
		h.sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status:      "error",
			Error:       "Only WAV format is supported",
			AudioFormat: filepath.Ext(header.Filename),
		})
		return
	}

	tmpPath := filepath.Join(h.uploadDir, header.Filename)
	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		h.logger.Printf("Failed to create upload dir: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Server error",
		})
		return
	}

	outFile, err := os.Create(tmpPath)
	if err != nil {
		h.logger.Printf("Failed to create temp file: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Server error",
		})
		return
	}

	_, err = io.Copy(outFile, file)
	outFile.Close()
	if err != nil {
		h.logger.Printf("Failed to save audio file: %v", err)
		os.Remove(tmpPath)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Failed to save audio file",
		})
		return
	}
	defer os.Remove(tmpPath)

	h.logger.Printf("Processing audio file: %s (%d bytes)", header.Filename, header.Size)

	text, err := h.asr.TranscribeWavFile(tmpPath)
	if err != nil {
		h.logger.Printf("ASR transcription failed: %v", err)
		h.sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status:      "error",
			Error:       fmt.Sprintf("Transcription failed: %v", err),
			AudioFormat: "wav",
		})
		return
	}

	confidence := estimateConfidence(text)
	h.logger.Printf("Transcription complete: %s (confidence: %.2f)", text, confidence)

	h.sendJSON(w, http.StatusOK, AsrResponse{
		Status:      "success",
		Confidence:  confidence,
		Text:        text,
		AudioFormat: "wav",
	})
}

func (h *AsrHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
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
