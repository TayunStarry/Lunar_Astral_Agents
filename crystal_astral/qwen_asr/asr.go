package qwen_asr

import (
	"CrystalAstral/qwen_asr/cpp"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// New 从模型目录加载 Qwen3-ASR 引擎
func New(modelDir string) (*QwenASR, error) {
	if os.Getenv("QWEN_BF16_CACHE_MB") == "" {
		os.Setenv("QWEN_BF16_CACHE_MB", defaultBf16CacheMB)
	}

	ctx := cpp.Load(modelDir)
	if ctx == nil {
		return nil, fmt.Errorf("failed to load model from %s", modelDir)
	}

	cpp.SetThreads(cpp.NumCPUs() / 2)

	return &QwenASR{
		ctx:      ctx,
		modelDir: modelDir,
	}, nil
}

// ensureEngine 懒加载引擎：首个请求到达时才加载模型，其后复用同一实例。
// 加载失败后需重启琉璃才能重试（与 Kokoro 引擎一致）。
func ensureEngine() (*QwenASR, error) {
	asrInitOnce.Do(func() {
		modelDir := *GeneralConfig.AsrModel
		LoggerGeneral.Info("QwenASR", "首次请求，加载 ASR 模型: %s", modelDir)

		asrEngine, asrInitErr = New(modelDir)
		if asrInitErr != nil {
			LoggerGeneral.Error("QwenASR", "ASR 引擎初始化失败: %v", asrInitErr)
			return
		}
		LoggerGeneral.Info("QwenASR", "ASR 引擎加载成功: %s", modelDir)
	})
	return asrEngine, asrInitErr
}

// Close 释放引擎资源
func (q *QwenASR) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx != nil {
		cpp.Free(q.ctx)
		q.ctx = nil
	}
}

// TranscribeWavFile 转写 WAV 文件，返回识别文本
func (q *QwenASR) TranscribeWavFile(wavPath string) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return "", fmt.Errorf("ASR context is closed")
	}

	text, ok := cpp.TranscribeFile(q.ctx, wavPath)
	if !ok {
		return "", fmt.Errorf("transcription failed for %s", wavPath)
	}
	return text, nil
}

// TranscribeAudioBuffer 转写 16kHz 单声道 float32 采样，返回识别文本
func (q *QwenASR) TranscribeAudioBuffer(samples []float32) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return "", fmt.Errorf("ASR context is closed")
	}
	if len(samples) == 0 {
		return "", fmt.Errorf("empty audio samples")
	}

	text, ok := cpp.TranscribeSamples(q.ctx, samples)
	if !ok {
		return "", fmt.Errorf("transcription failed")
	}
	return text, nil
}

// SetLanguage 设置强制识别语言
func (q *QwenASR) SetLanguage(lang string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return fmt.Errorf("ASR context is closed")
	}
	if !cpp.SetLanguage(q.ctx, lang) {
		return fmt.Errorf("unsupported language: %s", lang)
	}
	return nil
}

// SetPrompt 设置系统提示词
func (q *QwenASR) SetPrompt(prompt string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return fmt.Errorf("ASR context is closed")
	}
	if !cpp.SetPrompt(q.ctx, prompt) {
		return fmt.Errorf("failed to set prompt")
	}
	return nil
}

// SupportedLanguages 返回逗号分隔的支持语言列表
func SupportedLanguages() string {
	return cpp.SupportedLanguages()
}

// TranscribeBase64 转写 base64 编码的音频（供 LTP9 可视化测试节点复用）。
// WAV 直接识别；其他格式经 ffmpeg 转为 16kHz 单声道 WAV。返回识别文本与置信度。
func TranscribeBase64(audioBase64, srcExt string) (string, float64, error) {
	engine, err := ensureEngine()
	if err != nil {
		return "", 0, err
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(audioBase64))
	if err != nil {
		return "", 0, fmt.Errorf("音频 base64 解码失败: %w", err)
	}

	dir := filepath.Join(*GeneralConfig.LocalDir, "audios")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, fmt.Errorf("创建音频目录失败: %w", err)
	}

	var wavData []byte
	if isValidWav(raw) {
		wavData = raw
	} else {
		if srcExt == "" {
			srcExt = "wav"
		}
		wavData, err = convertToWav(raw, srcExt)
		if err != nil {
			return "", 0, err
		}
	}

	name := fmt.Sprintf("ltp9_asr_%d.wav", time.Now().UnixNano())
	tmpPath := filepath.Join(dir, name)
	if err := os.WriteFile(tmpPath, wavData, 0644); err != nil {
		return "", 0, fmt.Errorf("保存音频失败: %w", err)
	}
	defer os.Remove(tmpPath)

	text, err := engine.TranscribeWavFile(tmpPath)
	if err != nil {
		return "", 0, err
	}

	LoggerGeneral.Info("QwenASR", "lpt9 转写完成: %s (置信度: %.2f)", text, estimateConfidence(text))
	return text, estimateConfidence(text), nil
}
