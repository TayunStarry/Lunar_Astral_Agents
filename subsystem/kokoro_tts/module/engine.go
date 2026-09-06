package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logger "LunarSubsystem/LoggerGeneral"

	"github.com/yalue/onnxruntime_go"
)

// 模型文件名常量
const (
	modelFileName  = "Kokoro-82M-v1.1-zh.onnx"
	tokenizerFileN = "tokenizer.json"
	voicesDirName  = "voices"
	onnxRuntimeDll = "onnxruntime.dll"
	espeakDataDir  = "espeak-ng-data"
)

// defaultVoiceName 默认音色
const defaultVoiceName = "zf_001"

// SetOnnxLibraryPath 指定 onnxruntime 共享库路径（可选）
func SetOnnxLibraryPath(path string) {
	if path != "" {
		onnxruntime_go.SetSharedLibraryPath(path)
	}
}

// InitEngine 初始化 Kokoro TTS 引擎
// modelDir: 模型目录（含 .onnx 与 tokenizer.json），voicesDir: 音色目录，espeakDir: espeak-ng 数据目录（可为空）
func InitEngine(modelDir, voicesDir, espeakDir string) error {
	var initErr error
	initOnce.Do(func() {
		if err := onnxruntime_go.InitializeEnvironment(); err != nil {
			initErr = fmt.Errorf("初始化 onnxruntime 失败: %w", err)
			return
		}

		modelPath := filepath.Join(modelDir, modelFileName)
		if _, err := os.Stat(modelPath); err != nil {
			initErr = fmt.Errorf("模型文件不存在: %s", modelPath)
			return
		}
		// 校验模型输入输出契约
		if _, _, err := onnxruntime_go.GetInputOutputInfo(modelPath); err != nil {
			initErr = fmt.Errorf("模型加载校验失败: %w", err)
			return
		}

		session, err := onnxruntime_go.NewDynamicAdvancedSession(
			modelPath,
			[]string{"input_ids", "style", "speed"},
			[]string{"waveform", "duration"},
			nil,
		)
		if err != nil {
			initErr = fmt.Errorf("创建推理会话失败: %w", err)
			return
		}

		tokenizer, err := NewTokenizer(filepath.Join(modelDir, tokenizerFileN))
		if err != nil {
			initErr = err
			return
		}

		voices, order, err := LoadVoices(voicesDir)
		if err != nil {
			initErr = err
			return
		}

		zh, err := newZhFrontend()
		if err != nil {
			initErr = err
			return
		}

		// 加载用户读音词典（可运行时增删，持久化到模型目录）
		pronunciationDict, err = LoadPronunciationDict(filepath.Join(modelDir, "pronunciation_dict.json"))
		if err != nil {
			initErr = fmt.Errorf("加载读音词典失败: %w", err)
			return
		}

		espeakOK := initEspeak(espeakDir)

		globalEngine = &Engine{
			session:         session,
			tokenizer:       tokenizer,
			voices:          voices,
			voiceOrder:      order,
			zhFront:         zh,
			espeakAvailable: espeakOK,
		}
	})
	return initErr
}

// GetEngine 获取全局引擎实例
func GetEngine() *Engine {
	engineInitMu.Lock()
	defer engineInitMu.Unlock()
	return globalEngine
}

// ListVoices 返回可用音色列表
func (e *Engine) ListVoices() []VoiceInfo {
	infos := make([]VoiceInfo, 0, len(e.voiceOrder))
	for _, name := range e.voiceOrder {
		infos = append(infos, VoiceInfo{Name: name, Lang: voiceLang(name)})
	}
	return infos
}

// voiceLang 根据音色前缀判断语言
func voiceLang(name string) string {
	if strings.HasPrefix(name, "zf") || strings.HasPrefix(name, "zm") {
		return "zh"
	}
	return "en"
}

// Synthesize 文本转语音，返回 24kHz float32 采样与音素序列
func (e *Engine) Synthesize(text, voiceName string, speed float32, lang string) ([]float32, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", fmt.Errorf("文本内容不能为空")
	}
	if voiceName == "" {
		voiceName = defaultVoiceName
	}
	voice, ok := e.voices[voiceName]
	if !ok {
		return nil, "", fmt.Errorf("音色 %s 不存在，可用: %s", voiceName, strings.Join(e.voiceOrder[:min(10, len(e.voiceOrder))], ", "))
	}
	if speed == 0 {
		// 请求未指定语速时使用默认值
		speed = DefaultSpeed
	}
	if speed < MinSpeed {
		speed = MinSpeed
	}
	if speed > MaxSpeed {
		speed = MaxSpeed
	}

	// 1. 文本 -> 音素
	phonemes, err := e.phonemize(text, lang)
	if err != nil {
		return nil, "", err
	}
	if phonemes == "" {
		return nil, "", fmt.Errorf("文本未能生成音素")
	}

	// 2. 按上下文长度分块
	batches := splitPhonemes(phonemes, ContextLength)

	// 3. 逐块推理并拼接
	e.mu.Lock()
	defer e.mu.Unlock()

	var audio []float32
	for i, batch := range batches {
		if i > 0 {
			audio = append(audio, make([]float32, int(float32(SampleRate)*BatchPauseSeconds))...)
		}
		samples, err := e.runBatch(batch, voice, speed)
		if err != nil {
			return nil, phonemes, fmt.Errorf("第 %d 块合成失败: %w", i+1, err)
		}
		audio = append(audio, samples...)
	}
	if len(audio) == 0 {
		return nil, phonemes, fmt.Errorf("合成结果为空")
	}
	return audio, phonemes, nil
}

// runBatch 运行单块推理
func (e *Engine) runBatch(phonemes string, voice *Voice, speed float32) ([]float32, error) {
	tokens := e.tokenizer.Tokenize(phonemes)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("音素不在模型词表中")
	}
	// 首尾补 pad token 0
	inputData := make([]int64, 0, len(tokens)+2)
	inputData = append(inputData, 0)
	inputData = append(inputData, tokens...)
	inputData = append(inputData, 0)

	// 音色按音素数量选行
	style := styleFor(voice, len(tokens))

	inputTensor, err := onnxruntime_go.NewTensor(onnxruntime_go.Shape{1, int64(len(inputData))}, inputData)
	if err != nil {
		return nil, err
	}
	defer inputTensor.Destroy()

	styleTensor, err := onnxruntime_go.NewTensor(onnxruntime_go.Shape{1, StyleDim}, style)
	if err != nil {
		return nil, err
	}
	defer styleTensor.Destroy()

	speedTensor, err := onnxruntime_go.NewTensor(onnxruntime_go.Shape{1}, []float32{speed})
	if err != nil {
		return nil, err
	}
	defer speedTensor.Destroy()

	outputs := []onnxruntime_go.Value{nil, nil}
	if err := e.session.Run(
		[]onnxruntime_go.Value{inputTensor, styleTensor, speedTensor},
		outputs,
	); err != nil {
		return nil, err
	}

	wave, ok := outputs[0].(*onnxruntime_go.Tensor[float32])
	if !ok {
		// 类型断言失败时 wave 必为 nil；释放实际分配的输出对象避免泄漏
		if outputs[0] != nil {
			_ = outputs[0].Destroy()
		}
		return nil, fmt.Errorf("waveform 输出类型异常")
	}
	defer wave.Destroy()
	samples := wave.GetData()
	result := make([]float32, len(samples))
	copy(result, samples)
	return result, nil
}

// phonemize 文本转音素（自动中英路由）
func (e *Engine) phonemize(text, lang string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh":
		return e.zhFront.Call(numbersToChinese(text)), nil
	case "en":
		if !e.espeakAvailable {
			return "", fmt.Errorf("英文音素化不可用（espeak-ng 未就绪）")
		}
		return espeakPhonemize(text), nil
	}
	// 自动模式：中英混合分段
	var parts []string
	for _, seg := range splitEnZh(text) {
		if isEnglishSpan(seg) {
			if !e.espeakAvailable {
				continue
			}
			parts = append(parts, espeakPhonemize(seg))
		} else {
			parts = append(parts, e.zhFront.Call(numbersToChinese(seg)))
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("未能生成任何音素")
	}
	return strings.Join(parts, " "), nil
}

// splitPhonemes 将音素串按 510 长度分块，优先在标点处断开
func splitPhonemes(ps string, maxLen int) []string {
	if len(ps) <= maxLen {
		return []string{ps}
	}
	// 标点优先级：句末 > 停顿 > 轻停顿
	priorities := []string{"。！？.!?…", "，：；,:;", "、—"}
	var cut int
	for _, marks := range priorities {
		cut = lastIndexAnyBefore(ps, marks, maxLen)
		if cut > 0 {
			break
		}
	}
	if cut <= 0 {
		cut = maxLen
	}
	head := ps[:cut]
	rest := ps[cut:]
	head = strings.TrimRight(head, " ")
	rest = strings.TrimLeft(rest, " ")
	if head == "" {
		head = ps[:maxLen]
		rest = ps[maxLen:]
	}
	return append([]string{head}, splitPhonemes(rest, maxLen)...)
}

// lastIndexAnyBefore 在 [0, limit) 内查找最后一个指定字符的位置
func lastIndexAnyBefore(s, chars string, limit int) int {
	if limit > len(s) {
		limit = len(s)
	}
	for i := limit - 1; i >= 0; i-- {
		if strings.ContainsRune(chars, rune(s[i])) {
			return i + 1
		}
	}
	return -1
}

// splitEnZh 将文本划分为英文段与中文段
func splitEnZh(text string) []string {
	var segs []string
	var cur []rune
	curEn := false
	flush := func() {
		if len(cur) > 0 {
			segs = append(segs, string(cur))
			cur = nil
		}
	}
	for _, r := range text {
		isEn := r < 128 && (r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == ' ' || r == '\'' || r == '-')
		if len(cur) > 0 && isEn != curEn {
			flush()
		}
		curEn = isEn
		cur = append(cur, r)
	}
	flush()
	return segs
}

// isEnglishSpan 判断段落是否含英文字母（含字母则视为英文段）
func isEnglishSpan(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
}

// LogAvailable 输出引擎信息
func (e *Engine) LogAvailable() {
	logger.Info("KOKORO-TTS", "引擎就绪，音色 %d 个，示例: %s", len(e.voiceOrder), strings.Join(e.voiceOrder[:min(5, len(e.voiceOrder))], ", "))
}
