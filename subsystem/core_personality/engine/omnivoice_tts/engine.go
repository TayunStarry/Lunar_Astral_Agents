package OmniVoiceTTS

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	logger "LunarSubsystem/LoggerGeneral"

	"golang.org/x/sys/windows"
)

// ==== C ABI 结构镜像（见 omnivoice.h，字段布局须与 C 保持二进制一致）====

// cInitParams 对应 struct ov_init_params
type cInitParams struct {
	// abiVersion ABI 版本，必须置于偏移 0
	abiVersion int32
	// _ 对齐填充
	_ [4]byte
	// modelPath 语言模型 GGUF 路径
	modelPath unsafe.Pointer
	// codecPath 音频编解码器 GGUF 路径
	codecPath unsafe.Pointer
	// useFa GPU 后端启用 flash attention
	useFa bool
	// clampFp16 子 Ampere CUDA 的 FP16 累积保护
	clampFp16 bool
}

// cTTSParams 对应 struct ov_tts_params（v3）
type cTTSParams struct {
	// abiVersion ABI 版本，必须置于偏移 0
	abiVersion int32
	// _ 对齐填充
	_ [4]byte
	// text 待合成文本
	text unsafe.Pointer
	// lang 语言提示
	lang unsafe.Pointer
	// instruct 音色属性关键词
	instruct unsafe.Pointer
	// tOverride >0 强制单次合成的帧数
	tOverride int32
	// chunkDurationSec 分块时长（秒）
	chunkDurationSec float32
	// chunkThresholdSec 分块阈值（秒）
	chunkThresholdSec float32
	// denoise 参考音存在时输出 <|denoise|> 标记
	denoise bool
	// preprocessPrompt 对参考文本加标点、裁剪静音
	preprocessPrompt bool
	// _ 对齐填充
	_ [2]byte
	// mgNumStep MaskGIT 采样步数
	mgNumStep int32
	// mgGuidanceScale 引导强度
	mgGuidanceScale float32
	// mgTShift 温度偏移
	mgTShift float32
	// mgLayerPenaltyFactor 层惩罚因子
	mgLayerPenaltyFactor float32
	// mgPositionTemperature 位置温度
	mgPositionTemperature float32
	// mgClassTemperature 类别温度
	mgClassTemperature float32
	// mgSeed 随机种子
	mgSeed uint64
	// refAudioTokens 预编码参考音 RVQ 码矩阵 [K, ref_T]
	refAudioTokens unsafe.Pointer
	// refT 参考码帧数
	refT int32
	// _ 对齐填充
	_ [4]byte
	// refAudio24k 原始参考音 24kHz 单声道采样
	refAudio24k unsafe.Pointer
	// refNSamples 参考音采样数
	refNSamples int32
	// _ 对齐填充
	_ [4]byte
	// refText 参考音频转写文本
	refText unsafe.Pointer
	// dumpDir 中间张量导出目录
	dumpDir unsafe.Pointer
	// cancel 协作取消回调
	cancel unsafe.Pointer
	// cancelUserData 回调用户数据
	cancelUserData unsafe.Pointer
	// onChunk 流式输出回调
	onChunk unsafe.Pointer
	// onChunkUserData 流式回调用户数据
	onChunkUserData unsafe.Pointer
	// postproc 缓冲路径输出后处理开关
	postproc bool
	// _ 尾部对齐填充
	_ [7]byte
}

// cAudio 对应 struct ov_audio
type cAudio struct {
	// samples 指向单声道 float32 PCM（C 侧 malloc 分配，随 ov_audio_free 释放）
	samples unsafe.Pointer
	// nSamples 采样数
	nSamples int32
	// sampleRate 采样率
	sampleRate int32
	// channels 声道数
	channels int32
	// _ 对齐填充
	_ [4]byte
}

// ==== 动态库加载 ====

// resolveProcs 解析全部导出函数地址
func resolveProcs(dll windows.Handle) error {
	exports := []struct {
		name string
		dst  *uintptr
	}{
		{"ov_version", &ttsProcs.version},
		{"ov_last_error", &ttsProcs.lastError},
		{"ov_init_default_params", &ttsProcs.initDefaultParams},
		{"ov_init", &ttsProcs.init},
		{"ov_free", &ttsProcs.free},
		{"ov_tts_default_params", &ttsProcs.ttsDefaultParams},
		{"ov_synthesize", &ttsProcs.synthesize},
		{"ov_audio_free", &ttsProcs.audioFree},
		{"ov_n_languages", &ttsProcs.nLanguages},
		{"ov_language_id", &ttsProcs.languageID},
		{"ov_language_name", &ttsProcs.languageName},
	}
	for _, e := range exports {
		proc, err := windows.GetProcAddress(dll, e.name)
		if err != nil {
			return fmt.Errorf("解析导出函数 %s 失败: %w", e.name, err)
		}
		*e.dst = proc
	}
	return nil
}

// loadLibrary 从 modelDir 加载 omnivoice.dll 及其依赖
func loadLibrary(modelDir string) error {
	dllPath := filepath.Join(modelDir, DllName)
	if _, err := os.Stat(dllPath); err != nil {
		return fmt.Errorf("引擎库缺失: %s", dllPath)
	}
	absPath, err := filepath.Abs(dllPath)
	if err != nil {
		return fmt.Errorf("解析引擎库路径失败: %w", err)
	}
	dll, err := windows.LoadLibraryEx(absPath, 0, loadWithAlteredSearchPath)
	if err != nil {
		return fmt.Errorf("加载 %s 失败: %w", absPath, err)
	}
	if err := resolveProcs(dll); err != nil {
		windows.FreeLibrary(dll)
		return err
	}
	ttsProcs.dll = dll
	return nil
}

// ==== C ABI 调用辅助 ====

// cBytes 将 Go 字符串转为 NUL 结尾的 UTF-8 缓冲
func cBytes(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b
}

// cStringToGo 读取 C 侧返回的 NUL 结尾 UTF-8 字符串
func cStringToGo(s *byte) string {
	if s == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(s), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(s, n))
}

// ovLastError 读取最近一次引擎错误（通过 Go 侧变量取址解引用，规避 uintptr 直接转换）
func ovLastError() string {
	if ttsProcs.lastError == 0 {
		return ""
	}
	p, _, _ := syscall.SyscallN(ttsProcs.lastError)
	return cStringToGo(*(**byte)(unsafe.Pointer(&p)))
}

// ==== 引擎 ====

// refAudioBaseDirs 参考音频相对路径的附加解析基目录（由宿主注入，如 exe 目录下的 local_data）
var refAudioBaseDirs []string

// SetRefAudioBaseDirs 设置参考音频相对路径的附加解析基目录（须在 InitEngine 前调用）
func SetRefAudioBaseDirs(dirs []string) {
	refAudioBaseDirs = dirs
}

// resolveRefAudioPath 解析参考音频路径：
// 绝对路径原样返回；相对路径依次尝试「进程 cwd → 可执行文件目录 → 注入的基目录」，
// 兼容 "local_data/audios/x.wav"、"./audios/x.wav"、"audios/x.wav" 等写法
func resolveRefAudioPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	rel := p
	for _, prefix := range []string{"./", ".\\", "/"} {
		rel = strings.TrimPrefix(rel, prefix)
	}
	candidates := []string{p}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), rel))
	}
	for _, base := range refAudioBaseDirs {
		candidates = append(candidates, filepath.Join(base, rel))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// 全部未命中则返回原路径，交给读取方给出明确错误
	return p
}

// Engine OmniVoice TTS 引擎实例
type Engine struct {
	// handle ov_context 句柄
	handle uintptr
	// mu 保护合成调用的互斥锁（引擎非线程安全）
	mu sync.Mutex
	// modelPath 语言模型路径（日志/诊断用）
	modelPath string
	// version 引擎构建版本串
	version string
}

// InitEngine 懒初始化引擎：从 modelDir 加载 DLL 并创建推理上下文。
// 已加载直接返回；失败返回错误且不锁死（下次调用重试）。
// 模型目录应包含 omnivoice.dll、omnivoice-base-Q8_0.gguf、omnivoice-tokenizer-F32.gguf
func InitEngine(modelDir string) error {
	engineMu.Lock()
	defer engineMu.Unlock()
	if globalEngine != nil {
		return nil
	}
	return initEngine(modelDir)
}

// WithEngine 在读锁保护下访问引擎实例：合成期间引擎不会被卸载。
// 引擎未加载时返回错误（调用方应先 InitEngine）。
func WithEngine(fn func(e *Engine) error) error {
	engineMu.RLock()
	defer engineMu.RUnlock()
	if globalEngine == nil {
		return fmt.Errorf("OmniVoice 引擎未初始化")
	}
	return fn(globalEngine)
}

// UnloadEngine 卸载引擎：销毁推理上下文（模型权重随之释放）→ 释放 omnivoice.dll。
// 等待进行中的合成完成后才销毁（合成持有 engineMu 读锁）。未加载时为幂等空操作。
func UnloadEngine() error {
	engineMu.Lock()
	defer engineMu.Unlock()
	if globalEngine == nil {
		return nil
	}

	globalEngine.mu.Lock()
	if globalEngine.handle != 0 && ttsProcs.free != 0 {
		syscall.SyscallN(ttsProcs.free, globalEngine.handle)
	}
	globalEngine.mu.Unlock()

	globalEngine = nil
	freeLibrary()

	logger.Info("OmniVoiceTTS", "引擎已卸载（闲置释放，下次调用时重新加载）")
	return nil
}

// freeLibrary 释放 omnivoice.dll 模块并清空导出表（重载时由 loadLibrary 重新解析）
func freeLibrary() {
	if ttsProcs.dll != 0 {
		windows.FreeLibrary(ttsProcs.dll)
		ttsProcs.dll = 0
	}
	ttsProcs.version = 0
	ttsProcs.lastError = 0
	ttsProcs.initDefaultParams = 0
	ttsProcs.init = 0
	ttsProcs.free = 0
	ttsProcs.ttsDefaultParams = 0
	ttsProcs.synthesize = 0
	ttsProcs.audioFree = 0
	ttsProcs.nLanguages = 0
	ttsProcs.languageID = 0
	ttsProcs.languageName = 0
}

// initEngine 实际初始化流程（engineMu 写锁内调用）
func initEngine(modelDir string) error {
	if err := loadLibrary(modelDir); err != nil {
		return err
	}
	// 后续任一步骤失败都释放已加载的 DLL，避免失败重试累积模块引用计数
	defer func() {
		if globalEngine == nil {
			freeLibrary()
		}
	}()

	// 版本串（静态存储，进程生命周期内有效）
	var version string
	if ttsProcs.version != 0 {
		p, _, _ := syscall.SyscallN(ttsProcs.version)
		version = cStringToGo(*(**byte)(unsafe.Pointer(&p)))
	}

	modelPath := filepath.Join(modelDir, ModelFileName)
	codecPath := filepath.Join(modelDir, CodecFileName)
	for _, p := range []string{modelPath, codecPath} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("模型文件缺失: %s", p)
		}
	}

	var ip cInitParams
	syscall.SyscallN(ttsProcs.initDefaultParams, uintptr(unsafe.Pointer(&ip)))
	ip.abiVersion = abiVersion

	modelBytes := cBytes(modelPath)
	codecBytes := cBytes(codecPath)
	ip.modelPath = unsafe.Pointer(&modelBytes[0])
	ip.codecPath = unsafe.Pointer(&codecBytes[0])

	handle, _, _ := syscall.SyscallN(ttsProcs.init, uintptr(unsafe.Pointer(&ip)))
	// modelBytes/codecBytes 由 ip 的指针字段持有，GC 不会提前回收
	runtime.KeepAlive(ip)
	if handle == 0 {
		return fmt.Errorf("引擎初始化失败: %s", ovLastError())
	}

	globalEngine = &Engine{
		handle:    handle,
		modelPath: modelPath,
		version:   version,
	}
	return nil
}

// GetEngine 返回全局引擎实例（未初始化返回 nil）
func GetEngine() *Engine {
	return globalEngine
}

// IsLoaded 引擎是否已就绪
func IsLoaded() bool {
	return globalEngine != nil && globalEngine.handle != 0
}

// Version 返回引擎构建版本串
func (e *Engine) Version() string { return e.version }

// Synthesize 执行语音合成，返回 24kHz 单声道 float32 采样。
// 调用方须持有 engineMu 读锁（WithEngine），防止合成中途引擎被卸载。
func (e *Engine) Synthesize(req TTSRequest) ([]float32, error) {
	// 更新最近合成时间戳（闲置卸载判据）
	touchEngine()
	if e == nil || e.handle == 0 {
		return nil, fmt.Errorf("引擎未初始化")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 可选参考音（声音克隆）：任意采样率单/双声道 WAV → 24kHz 单声道
	var refSamples []float32
	if req.RefAudio != "" {
		var err error
		refSamples, err = loadWAVMono24k(resolveRefAudioPath(req.RefAudio))
		if err != nil {
			return nil, fmt.Errorf("参考音频读取失败: %w", err)
		}
		if len(refSamples) == 0 {
			return nil, fmt.Errorf("参考音频为空: %s", req.RefAudio)
		}
	}

	var params cTTSParams
	syscall.SyscallN(ttsProcs.ttsDefaultParams, uintptr(unsafe.Pointer(&params)))
	params.abiVersion = abiVersion

	textBytes := cBytes(req.Text)
	langBytes := cBytes(req.Lang)
	instructBytes := cBytes(req.Instruct)
	refTextBytes := cBytes(req.RefText)

	params.text = unsafe.Pointer(&textBytes[0])
	params.lang = unsafe.Pointer(&langBytes[0])
	params.instruct = unsafe.Pointer(&instructBytes[0])
	params.refText = unsafe.Pointer(&refTextBytes[0])
	params.postproc = !req.DisablePostproc
	if req.Seed != 0 {
		params.mgSeed = req.Seed
	}
	if len(refSamples) > 0 {
		params.refAudio24k = unsafe.Pointer(&refSamples[0])
		params.refNSamples = int32(len(refSamples))
	}

	var out cAudio
	rc, _, _ := syscall.SyscallN(
		ttsProcs.synthesize,
		e.handle,
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Pointer(&out)),
	)
	// 保持全部字符串缓冲与参考音存活到合成结束
	runtime.KeepAlive(params)
	runtime.KeepAlive(textBytes)
	runtime.KeepAlive(langBytes)
	runtime.KeepAlive(instructBytes)
	runtime.KeepAlive(refTextBytes)
	runtime.KeepAlive(refSamples)
	if rc != 0 {
		// 失败时 out 可能仍持有分配，稳妥释放
		syscall.SyscallN(ttsProcs.audioFree, uintptr(unsafe.Pointer(&out)))
		return nil, fmt.Errorf("合成失败 (status=%d): %s", int32(rc), ovLastError())
	}

	n := int(out.nSamples)
	if n == 0 || out.samples == nil {
		syscall.SyscallN(ttsProcs.audioFree, uintptr(unsafe.Pointer(&out)))
		return nil, fmt.Errorf("合成结果为空")
	}

	samples := make([]float32, n)
	copy(samples, unsafe.Slice((*float32)(out.samples), n))
	syscall.SyscallN(ttsProcs.audioFree, uintptr(unsafe.Pointer(&out)))
	return samples, nil
}

// Languages 返回支持的语言列表
func Languages() []LanguageEntry {
	if ttsProcs.nLanguages == 0 {
		return nil
	}
	n, _, _ := syscall.SyscallN(ttsProcs.nLanguages)
	list := make([]LanguageEntry, 0, int(n))
	for i := 0; i < int(n); i++ {
		idPtr, _, _ := syscall.SyscallN(ttsProcs.languageID, uintptr(i))
		namePtr, _, _ := syscall.SyscallN(ttsProcs.languageName, uintptr(i))
		list = append(list, LanguageEntry{
			Id:   cStringToGo(*(**byte)(unsafe.Pointer(&idPtr))),
			Name: cStringToGo(*(**byte)(unsafe.Pointer(&namePtr))),
		})
	}
	return list
}
