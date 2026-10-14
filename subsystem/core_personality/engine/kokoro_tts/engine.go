package KokoroTTS

import (
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// InitEngine 初始化 Kokoro TTS 引擎（惰性加载：已加载直接返回；失败返回错误且不锁死，下次调用重试）
// modelDir: 模型目录（含 .onnx 与 tokenizer.json），voicesDir: 音色目录，espeakDir: espeak-ng 数据目录（可为空）
func InitEngine(modelDir, voicesDir, espeakDir string) error {
	engineInitMu.Lock()
	defer engineInitMu.Unlock()
	if globalEngine != nil {
		return nil
	}

	// ONNX Runtime 默认把后端探测与图分区提示直写 stderr，而宿主 TUI 独占终端屏幕，
	// 故将环境日志级别提高到 ERROR：常规提示不再输出，真实错误仍经返回错误进日志管线
	if err := onnxruntime_go.InitializeEnvironment(onnxruntime_go.WithLogLevelError()); err != nil {
		return fmt.Errorf("初始化 onnxruntime 失败: %w", err)
	}

	modelPath := filepath.Join(modelDir, modelFileName)
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("模型文件不存在: %s", modelPath)
	}
	// 校验模型输入输出契约
	if _, _, err := onnxruntime_go.GetInputOutputInfo(modelPath); err != nil {
		return fmt.Errorf("模型加载校验失败: %w", err)
	}

	// 会话级日志同样收敛到 ERROR（环境级别不覆盖会话自身的输出）
	sessionOpts, err := onnxruntime_go.NewSessionOptions()
	if err != nil {
		return fmt.Errorf("创建会话选项失败: %w", err)
	}
	if err := sessionOpts.SetLogSeverityLevel(onnxruntime_go.LoggingLevelError); err != nil {
		_ = sessionOpts.Destroy()
		return fmt.Errorf("设置会话日志级别失败: %w", err)
	}
	session, err := onnxruntime_go.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input_ids", "style", "speed"},
		[]string{"waveform", "duration"},
		sessionOpts,
	)
	// 会话创建后选项即不再需要（ORT 在 CreateSession 时已复制所需配置）
	_ = sessionOpts.Destroy()
	if err != nil {
		return fmt.Errorf("创建推理会话失败: %w", err)
	}

	tokenizer, err := NewTokenizer(filepath.Join(modelDir, tokenizerFileN))
	if err != nil {
		return err
	}

	voices, order, err := LoadVoices(voicesDir)
	if err != nil {
		return err
	}

	zh, err := newZhFrontend()
	if err != nil {
		return err
	}

	// 加载用户读音词典（可运行时增删，持久化到模型目录）
	pronunciationDict, err = LoadPronunciationDict(filepath.Join(modelDir, "pronunciation_dict.json"))
	if err != nil {
		return fmt.Errorf("加载读音词典失败: %w", err)
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
	return nil
}

// WithEngine 在读锁保护下访问引擎实例：合成期间引擎不会被卸载。
// 引擎未加载时返回错误（调用方应先 InitEngine）。
func WithEngine(fn func(e *Engine) error) error {
	engineInitMu.RLock()
	defer engineInitMu.RUnlock()
	if globalEngine == nil {
		return fmt.Errorf("Kokoro 引擎未初始化")
	}
	return fn(globalEngine)
}

// UnloadEngine 卸载引擎：销毁 ONNX 会话（模型权重随之释放）并置空实例。
// 等待进行中的合成完成后才销毁（合成持有 engineInitMu 读锁）。未加载时为幂等空操作。
// 注：onnxruntime 运行时库无法随会话一并卸载（onnxruntime_go 不提供环境反初始化），仅释放模型权重。
func UnloadEngine() error {
	engineInitMu.Lock()
	defer engineInitMu.Unlock()
	if globalEngine == nil {
		return nil
	}

	globalEngine.mu.Lock()
	if globalEngine.session != nil {
		_ = globalEngine.session.Destroy()
	}
	globalEngine.mu.Unlock()

	globalEngine = nil
	LoggerGeneral.Info("KokoroTTS", "引擎已卸载（闲置释放，下次调用时重新加载）")
	return nil
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
// mix 提供 ≥2 个音色时启用音色融合（加权平均 style 向量），优先于 voiceName
// 调用方须持有 engineInitMu 读锁（WithEngine），防止合成中途引擎被卸载
func (e *Engine) Synthesize(text, voiceName string, speed float32, lang string, mix []MixVoice) ([]float32, string, error) {
	// 更新最近合成时间戳（闲置卸载判据）
	touchEngine()
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", fmt.Errorf("文本内容不能为空")
	}
	if voiceName == "" {
		voiceName = defaultVoiceName
	}
	var voice *Voice
	var mixWeights []voiceWeight
	if len(mix) > 0 {
		// 音色融合：校验音色存在并归一化权重
		var total float64
		for _, m := range mix {
			v, ok := e.voices[m.Voice]
			if !ok {
				return nil, "", fmt.Errorf("混合音色 %s 不存在，可用: %s", m.Voice, strings.Join(e.voiceOrder[:min(10, len(e.voiceOrder))], ", "))
			}
			if m.Weight < 0 {
				m.Weight = 0
			}
			total += m.Weight
			mixWeights = append(mixWeights, voiceWeight{voice: v, weight: float32(m.Weight)})
		}
		if len(mixWeights) < 2 {
			return nil, "", fmt.Errorf("音色混合至少需要 2 个音色")
		}
		if total <= 0 {
			// 权重全为 0：等权混合
			for i := range mixWeights {
				mixWeights[i].weight = 1
			}
			total = float64(len(mixWeights))
		}
		for i := range mixWeights {
			mixWeights[i].weight /= float32(total)
		}
	} else {
		var ok bool
		voice, ok = e.voices[voiceName]
		if !ok {
			return nil, "", fmt.Errorf("音色 %s 不存在，可用: %s", voiceName, strings.Join(e.voiceOrder[:min(10, len(e.voiceOrder))], ", "))
		}
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

	// 1. 滤除不可合成字符（Emoji、控制符、格式符等）
	cleaned := sanitizeText(text)
	if removed := len([]rune(text)) - len([]rune(cleaned)); removed > 0 {
		LoggerGeneral.SubInfo("KOKORO-TTS", "TEXT", "滤除 %d 个不可合成字符", removed)
	}
	if strings.TrimSpace(cleaned) == "" {
		return nil, "", fmt.Errorf("净化后文本内容不能为空")
	}

	// 2. 解析韵律提示符并逐段音素化
	//    [↑] 升调 / [↓] 降调（作用于其前文本段）、[•N] 停顿 N*0.01 秒
	segs := parseProsody(cleaned)
	var units []prosodyUnit
	var phonemeParts []string
	for _, s := range segs {
		if strings.TrimSpace(s.Text) == "" {
			// 纯停顿段
			units = append(units, prosodyUnit{pauseAfter: s.PauseAfter})
			continue
		}
		phonemes, err := e.phonemize(s.Text, lang)
		if err != nil {
			return nil, "", err
		}
		if phonemes == "" {
			units = append(units, prosodyUnit{pauseAfter: s.PauseAfter})
			continue
		}
		phonemeParts = append(phonemeParts, phonemes)
		units = append(units, prosodyUnit{phonemes: phonemes, rise: s.Rise, fall: s.Fall, pauseAfter: s.PauseAfter})
	}
	if len(units) == 0 {
		return nil, "", fmt.Errorf("文本未能生成音素")
	}

	// 3. 逐段推理并拼接（onnx 会话非并发安全，需持锁）
	e.mu.Lock()
	defer e.mu.Unlock()

	var audio []float32
	for _, u := range units {
		if u.phonemes == "" {
			// 纯停顿段：直接插入静音
			audio = append(audio, make([]float32, pauseSamples(u.pauseAfter))...)
			continue
		}
		segAudio, err := e.synthesizePhonemes(u.phonemes, voice, speed, mixWeights)
		if err != nil {
			return nil, strings.Join(phonemeParts, " "), err
		}
		// 应用升调/降调（音高偏移，保持时长不变）
		switch {
		case u.rise:
			segAudio = pitchShift(segAudio, RiseFactor)
		case u.fall:
			segAudio = pitchShift(segAudio, FallFactor)
		}
		audio = append(audio, segAudio...)
		if u.pauseAfter > 0 {
			audio = append(audio, make([]float32, pauseSamples(u.pauseAfter))...)
		}
	}
	if len(audio) == 0 {
		return nil, strings.Join(phonemeParts, " "), fmt.Errorf("合成结果为空")
	}
	return audio, strings.Join(phonemeParts, " "), nil
}

// synthesizePhonemes 将音素串按上下文长度分块并逐块推理拼接（需在 e.mu 保护下调用）
func (e *Engine) synthesizePhonemes(phonemes string, voice *Voice, speed float32, mix []voiceWeight) ([]float32, error) {
	batches := splitPhonemes(phonemes, ContextLength)

	var audio []float32
	for i, batch := range batches {
		if i > 0 {
			audio = append(audio, make([]float32, int(float32(SampleRate)*BatchPauseSeconds))...)
		}
		samples, err := e.runBatch(batch, voice, speed, mix)
		if err != nil {
			return nil, fmt.Errorf("第 %d 块合成失败: %w", i+1, err)
		}
		audio = append(audio, samples...)
	}
	return audio, nil
}

// voiceWeight 音色混合分量（已解析的音色引用 + 归一化权重）
type voiceWeight struct {
	// voice 音色引用
	voice *Voice
	// weight 归一化后的融合权重（和为 1）
	weight float32
}

// runBatch 运行单块推理
func (e *Engine) runBatch(phonemes string, voice *Voice, speed float32, mix []voiceWeight) ([]float32, error) {
	tokens := e.tokenizer.Tokenize(phonemes)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("音素不在模型词表中")
	}
	// 首尾补 pad token 0
	inputData := make([]int64, 0, len(tokens)+2)
	inputData = append(inputData, 0)
	inputData = append(inputData, tokens...)
	inputData = append(inputData, 0)

	// 音色按音素数量选行；混合时对各音色行做加权平均
	var style []float32
	if len(mix) > 0 {
		style = make([]float32, StyleDim)
		for _, mw := range mix {
			row := styleFor(mw.voice, len(tokens))
			for j := 0; j < StyleDim; j++ {
				style[j] += row[j] * mw.weight
			}
		}
	} else {
		style = styleFor(voice, len(tokens))
	}

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
	// 释放未被使用的 duration 输出张量（当前合成不取用，及时销毁避免资源泄漏）
	if outputs[1] != nil {
		_ = outputs[1].Destroy()
	}
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
	LoggerGeneral.Info("KOKORO-TTS", "引擎就绪，音色 %d 个，示例: %s", len(e.voiceOrder), strings.Join(e.voiceOrder[:min(5, len(e.voiceOrder))], ", "))
}
