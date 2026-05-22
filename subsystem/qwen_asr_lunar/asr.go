package main

/*
#cgo CFLAGS: -Wall -O3 -march=native -ffast-math
#cgo LDFLAGS: -lm -lpthread

#include "qwen_asr.h"
#include "qwen_asr_audio.h"
#include "qwen_asr_kernels.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

type QwenASR struct {
	ctx      *C.qwen_ctx_t
	mu       sync.Mutex
	modelDir string
}

func New(modelDir string) (*QwenASR, error) {
	cModelDir := C.CString(modelDir)
	defer C.free(unsafe.Pointer(cModelDir))

	ctx := C.qwen_load(cModelDir)
	if ctx == nil {
		return nil, fmt.Errorf("failed to load model from %s", modelDir)
	}

	return &QwenASR{
		ctx:      ctx,
		modelDir: modelDir,
	}, nil
}

func (q *QwenASR) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ctx != nil {
		C.qwen_free(q.ctx)
		q.ctx = nil
	}
}

func (q *QwenASR) TranscribeWavFile(wavPath string) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return "", fmt.Errorf("ASR context is closed")
	}

	cWavPath := C.CString(wavPath)
	defer C.free(unsafe.Pointer(cWavPath))

	cText := C.qwen_transcribe(q.ctx, cWavPath)
	if cText == nil {
		return "", fmt.Errorf("transcription failed for %s", wavPath)
	}
	defer C.free(unsafe.Pointer(cText))

	return C.GoString(cText), nil
}

func (q *QwenASR) TranscribeAudioBuffer(samples []float32) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return "", fmt.Errorf("ASR context is closed")
	}

	if len(samples) == 0 {
		return "", fmt.Errorf("empty audio samples")
	}

	cSamples := (*C.float)(unsafe.Pointer(&samples[0]))
	cNSamples := C.int(len(samples))

	cText := C.qwen_transcribe_audio(q.ctx, cSamples, cNSamples)
	if cText == nil {
		return "", fmt.Errorf("transcription failed")
	}
	defer C.free(unsafe.Pointer(cText))

	return C.GoString(cText), nil
}

func (q *QwenASR) SetLanguage(lang string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return fmt.Errorf("ASR context is closed")
	}

	cLang := C.CString(lang)
	defer C.free(unsafe.Pointer(cLang))

	ret := C.qwen_set_force_language(q.ctx, cLang)
	if ret != 0 {
		return fmt.Errorf("unsupported language: %s", lang)
	}
	return nil
}

func (q *QwenASR) SetPrompt(prompt string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.ctx == nil {
		return fmt.Errorf("ASR context is closed")
	}

	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))

	ret := C.qwen_set_prompt(q.ctx, cPrompt)
	if ret != 0 {
		return fmt.Errorf("failed to set prompt")
	}
	return nil
}

func SupportedLanguages() string {
	return C.GoString(C.qwen_supported_languages_csv())
}
