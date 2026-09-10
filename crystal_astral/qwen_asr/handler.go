package qwen_asr

import (
	"LunarSubsystem/GeneralConfig"
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

// Handler ASR 端点处理函数：POST multipart/form-data（audio 字段为音频文件）→ 识别文本 JSON
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSON(w, http.StatusMethodNotAllowed, AsrResponse{
			Status: "error",
			Error:  "Only POST method is allowed",
		})
		return
	}

	engine, err := ensureEngine()
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  fmt.Sprintf("ASR 引擎初始化失败: %v", err),
		})
		return
	}

	handleAsr(w, r, engine)
}

// handleAsr 解析音频表单、必要时转为 WAV，然后执行识别
func handleAsr(w http.ResponseWriter, r *http.Request, engine *QwenASR) {
	LoggerGeneral.Info("QwenASR", "收到语音识别请求")

	err := r.ParseMultipartForm(maxUploadMemory)
	if err != nil {
		LoggerGeneral.Error("QwenASR", "解析表单失败: %v", err)
		sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status: "error",
			Error:  "Failed to parse form data",
		})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		LoggerGeneral.Error("QwenASR", "获取音频文件失败: %v", err)
		sendJSON(w, http.StatusBadRequest, AsrResponse{
			Status: "error",
			Error:  "Missing audio file (use form field 'audio')",
		})
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		LoggerGeneral.Error("QwenASR", "读取音频数据失败: %v", err)
		sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Failed to read audio data",
		})
		return
	}

	dir := filepath.Join(*GeneralConfig.LocalDir, "audios")
	if err := os.MkdirAll(dir, 0755); err != nil {
		LoggerGeneral.Error("QwenASR", "创建上传目录失败: %v", err)
		sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Server error",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	tmpPath := filepath.Join(dir, header.Filename)

	var wavData []byte

	if isValidWav(audioData) {
		wavData = audioData
	} else {
		LoggerGeneral.Info("QwenASR", "检测到非 WAV 格式（文件名: %s），转换为 WAV", header.Filename)
		srcExt := ext
		if srcExt != ".webm" && srcExt != ".ogg" && srcExt != ".mp4" && srcExt != ".m4a" && srcExt != ".weba" {
			srcExt = ".webm"
		}
		wavData, err = convertToWav(audioData, srcExt)
		if err != nil {
			LoggerGeneral.Error("QwenASR", "音频转换失败: %v", err)
			sendJSON(w, http.StatusInternalServerError, AsrResponse{
				Status: "error",
				Error:  fmt.Sprintf("Audio conversion failed: %v", err),
			})
			return
		}
		tmpPath = filepath.Join(dir, "converted_"+header.Filename+".wav")
	}

	if err := os.WriteFile(tmpPath, wavData, 0644); err != nil {
		LoggerGeneral.Error("QwenASR", "保存音频文件失败: %v", err)
		sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status: "error",
			Error:  "Failed to save audio file",
		})
		return
	}
	defer os.Remove(tmpPath)

	LoggerGeneral.Info("QwenASR", "开始识别音频: %s (%d 字节, 格式=%s)", header.Filename, len(wavData), ext)

	text, err := engine.TranscribeWavFile(tmpPath)
	if err != nil {
		LoggerGeneral.Error("QwenASR", "语音识别失败: %v", err)
		sendJSON(w, http.StatusInternalServerError, AsrResponse{
			Status:      "error",
			Error:       fmt.Sprintf("Transcription failed: %v", err),
			AudioFormat: ext,
		})
		return
	}

	confidence := estimateConfidence(text)
	LoggerGeneral.Info("QwenASR", "识别完成: %s (置信度: %.2f)", text, confidence)

	sendJSON(w, http.StatusOK, AsrResponse{
		Status:      "success",
		Confidence:  confidence,
		Text:        text,
		AudioFormat: "wav",
	})
}

// isValidWav 判断数据是否为 WAV（RIFF....WAVE）
func isValidWav(data []byte) bool {
	if len(data) < 44 {
		return false
	}
	return data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'A' && data[10] == 'V' && data[11] == 'E'
}

// convertToWav 调用 ffmpeg 将任意音频转为 16kHz 单声道 PCM WAV
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

// findFfmpeg 查找 ffmpeg 可执行文件
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

// sendJSON 写入 JSON 响应
func sendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// estimateConfidence 按文本长度估算置信度
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
