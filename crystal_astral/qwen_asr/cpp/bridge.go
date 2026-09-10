// Package cpp 是 Qwen3-ASR 纯 C 推理引擎的 cgo 绑定层。
// 本目录内的 *.c/*.h 为引擎全部 C 源码：cgo 只会编译与 import "C" 的 Go 文件同目录的 C 源码，
// 因此绑定文件必须与 C 源码同目录，Go 侧（CrystalAstral/qwen_asr）通过本包调用引擎。
package cpp

/*
#cgo CFLAGS: -O3 -march=native -mtune=native -ffast-math -fopenmp -DUSE_BLAS
#cgo CFLAGS: -I${SRCDIR}/../openblas/include
#cgo LDFLAGS: -fopenmp -lm -lpthread
#cgo LDFLAGS: -L${SRCDIR}/../openblas/lib -l:libopenblas.a

#include "qwen_asr.h"
#include "qwen_asr_audio.h"
#include "qwen_asr_kernels.h"
#include <stdlib.h>
*/
import "C"

import "unsafe"

// Load 从模型目录加载引擎，成功返回引擎上下文（C qwen_ctx_t*），失败返回 nil。
func Load(modelDir string) unsafe.Pointer {
	cModelDir := C.CString(modelDir)
	defer C.free(unsafe.Pointer(cModelDir))
	return unsafe.Pointer(C.qwen_load(cModelDir))
}

// Free 释放引擎上下文及其占用的全部资源。
func Free(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}
	C.qwen_free((*C.qwen_ctx_t)(ctx))
}

// NumCPUs 返回可用 CPU 核心数。
func NumCPUs() int {
	return int(C.qwen_get_num_cpus())
}

// SetThreads 设置并行推理线程数。
func SetThreads(n int) {
	C.qwen_set_threads(C.int(n))
}

// TranscribeFile 转写 WAV 文件，返回识别文本；失败返回 ok=false。
func TranscribeFile(ctx unsafe.Pointer, wavPath string) (string, bool) {
	cWavPath := C.CString(wavPath)
	defer C.free(unsafe.Pointer(cWavPath))

	cText := C.qwen_transcribe((*C.qwen_ctx_t)(ctx), cWavPath)
	if cText == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(cText))

	return C.GoString(cText), true
}

// TranscribeSamples 转写 16kHz 单声道 float32 采样，返回识别文本；失败返回 ok=false。
func TranscribeSamples(ctx unsafe.Pointer, samples []float32) (string, bool) {
	if len(samples) == 0 {
		return "", false
	}

	cText := C.qwen_transcribe_audio(
		(*C.qwen_ctx_t)(ctx),
		(*C.float)(unsafe.Pointer(&samples[0])),
		C.int(len(samples)),
	)
	if cText == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(cText))

	return C.GoString(cText), true
}

// SetLanguage 设置强制识别语言，语言不受支持时返回 false。
func SetLanguage(ctx unsafe.Pointer, lang string) bool {
	cLang := C.CString(lang)
	defer C.free(unsafe.Pointer(cLang))
	return C.qwen_set_force_language((*C.qwen_ctx_t)(ctx), cLang) == 0
}

// SetPrompt 设置系统提示词，失败返回 false。
func SetPrompt(ctx unsafe.Pointer, prompt string) bool {
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	return C.qwen_set_prompt((*C.qwen_ctx_t)(ctx), cPrompt) == 0
}

// SupportedLanguages 返回逗号分隔的支持语言列表。
func SupportedLanguages() string {
	return C.GoString(C.qwen_supported_languages_csv())
}
