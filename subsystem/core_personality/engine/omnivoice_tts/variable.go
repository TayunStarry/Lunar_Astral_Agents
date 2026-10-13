package OmniVoiceTTS

import (
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

// ==== 常量 ====

// SampleRate 输出采样率（OmniVoice 固定 24kHz 单声道）
const SampleRate = 24000

// DllName 引擎动态库名（置于模型目录）
const DllName = "omnivoice.dll"

// ModelFileName 语言模型 GGUF 文件名
const ModelFileName = "omnivoice-base-Q8_0.gguf"

// CodecFileName 音频编解码器 GGUF 文件名
const CodecFileName = "omnivoice-tokenizer-F32.gguf"

// loadWithAlteredSearchPath 对应 LoadLibraryEx 的 LOAD_WITH_ALTERED_SEARCH_PATH 标志，
// 使依赖 DLL（vulkan-1/libgomp-1/libwinpthread-1）自动从引擎库同目录解析
const loadWithAlteredSearchPath = 0x0008

// abiVersion 当前绑定的公共 ABI 版本（见 omnivoice.h 的 OV_ABI_VERSION）
const abiVersion = 3

// ==== 全局状态 ====

// globalEngine 全局引擎实例
var globalEngine *Engine

// engineMu 引擎生命周期锁：
//   - 写锁（Lock）串行化加载（InitEngine）与卸载（UnloadEngine）
//   - 读锁（RLock）在合成期间钉住引擎实例（WithEngine），防止合成中途被卸载
var engineMu sync.RWMutex

// lastUsedUnix 最近一次合成调用的 unix 秒（0 = 从未合成；供闲置卸载守护判断）
var lastUsedUnix atomic.Int64

// touchEngine 更新最近合成时间戳（合成入口调用）
func touchEngine() {
	lastUsedUnix.Store(time.Now().Unix())
}

// LastUsedUnix 最近一次合成调用的 unix 秒（0 = 从未合成）
func LastUsedUnix() int64 {
	return lastUsedUnix.Load()
}

// ==== 导出函数指针表 ====

// ttsProcs 存储 omnivoice.dll 导出函数地址
var ttsProcs struct {
	// dll 为 omnivoice.dll 的模块句柄
	dll windows.Handle
	// version 对应 ov_version
	version uintptr
	// lastError 对应 ov_last_error
	lastError uintptr
	// initDefaultParams 对应 ov_init_default_params
	initDefaultParams uintptr
	// init 对应 ov_init
	init uintptr
	// free 对应 ov_free
	free uintptr
	// ttsDefaultParams 对应 ov_tts_default_params
	ttsDefaultParams uintptr
	// synthesize 对应 ov_synthesize
	synthesize uintptr
	// audioFree 对应 ov_audio_free
	audioFree uintptr
	// nLanguages 对应 ov_n_languages
	nLanguages uintptr
	// languageID 对应 ov_language_id
	languageID uintptr
	// languageName 对应 ov_language_name
	languageName uintptr
}
