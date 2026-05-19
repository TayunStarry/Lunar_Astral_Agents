package tts

/*
#cgo LDFLAGS: -L"D:/TTS/qwen3-tts.cpp-main/build" -L"D:/TTS/qwen3-tts.cpp-main/ggml/build/src" "D:/TTS/qwen3-tts.cpp-main/build/libqwen3tts.dll.a" -lqwen3_tts -ltts_transformer -ltext_tokenizer -laudio_tokenizer_encoder -laudio_tokenizer_decoder "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-base.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-cpu.a" -lstdc++ -lpthread
#cgo CFLAGS: -I"D:/TTS/qwen3-tts.cpp-main/src" -I"D:/TTS/qwen3-tts.cpp-main/ggml/include"
#include <stdlib.h>
#include "qwen3tts_c_api.h"
*/
import "C"

import (
	"config"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"unsafe"
)

func (c *TTSCache) Get(text string) (*cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.items[text]
	if exists {
		for i, t := range c.order {
			if t == text {
				c.order = append(c.order[:i], c.order[i+1:]...)
				c.order = append(c.order, text)
				break
			}
		}
		return entry, true
	}
	return nil, false
}

func (c *TTSCache) GetOrSetPending(text string) (*cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[text]; exists {
		return entry, false
	}

	if len(c.order) >= maxCacheSize {
		oldest := c.order[0]
		delete(c.items, oldest)
		c.order = c.order[1:]
	}

	entry := &cacheEntry{
		ready: make(chan struct{}),
	}
	c.items[text] = entry
	c.order = append(c.order, text)
	return entry, true
}

func (c *TTSCache) Remove(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, text)
	for i, t := range c.order {
		if t == text {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

func (e *cacheEntry) MarkReady(audio string) {
	e.audio = audio
	close(e.ready)
}

func (e *cacheEntry) Wait() string {
	<-e.ready
	return e.audio
}

func InitTTSEngine() {
	ttsOnce.Do(func() {
		modelDir := *config.LocalDir + "/models"
		refAudio := *config.LocalDir + "/audios/lunar-template.wav"
		cModelDir := C.CString(modelDir)
		defer C.free(unsafe.Pointer(cModelDir))
		nThreads := max(1, runtime.NumCPU()-1)
		handle := C.qwen3_tts_create(cModelDir, C.int32_t(nThreads))
		if handle == nil {
			log.Printf("Qwen TTS 引擎初始化失败，模型目录: %s", modelDir)
			return
		}
		globalTTS = &TTSEngine{
			handle:     handle,
			modelDir:   modelDir,
			refAudio:   refAudio,
			languageID: 2055,
		}
		log.Printf("Qwen TTS 引擎初始化成功，使用线程数: %d", nThreads)
	})
}

func synthesizeText(text string) ([]float32, error) {
	globalTTS.mu.Lock()
	defer globalTTS.mu.Unlock()

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cRefAudio := C.CString(globalTTS.refAudio)
	defer C.free(unsafe.Pointer(cRefAudio))

	var cParams C.Qwen3TtsParams
	C.qwen3_tts_default_params(&cParams)
	cParams.n_threads = 4
	cParams.language_id = C.int32_t(globalTTS.languageID)

	result := C.qwen3_tts_synthesize_with_voice_file(
		globalTTS.handle,
		cText,
		cRefAudio,
		&cParams,
	)

	if result == nil {
		errStr := C.GoString(C.qwen3_tts_get_error(globalTTS.handle))
		return nil, fmt.Errorf("TTS 合成失败: %s", errStr)
	}

	defer C.qwen3_tts_free_audio(result)

	nSamples := int(result.n_samples)
	if nSamples == 0 {
		return nil, fmt.Errorf("TTS 合成结果为空")
	}

	samples := make([]float32, nSamples)
	cSamples := (*[1 << 30]C.float)(unsafe.Pointer(result.samples))[:nSamples:nSamples]
	for i := 0; i < nSamples; i++ {
		samples[i] = float32(cSamples[i])
	}

	return samples, nil
}

func encodePCMToWAV(samples []float32, sampleRate int) []byte {
	numSamples := len(samples)
	byteRate := sampleRate * 2
	blockAlign := 2
	dataSize := numSamples * 2
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)

	buf[0] = 'R'
	buf[1] = 'I'
	buf[2] = 'F'
	buf[3] = 'F'
	putUint32LE(buf[4:], uint32(fileSize))
	buf[8] = 'W'
	buf[9] = 'A'
	buf[10] = 'V'
	buf[11] = 'E'
	buf[12] = 'f'
	buf[13] = 'm'
	buf[14] = 't'
	buf[15] = ' '
	putUint32LE(buf[16:], 16)
	putUint16LE(buf[20:], 1)
	putUint16LE(buf[22:], 1)
	putUint32LE(buf[24:], uint32(sampleRate))
	putUint32LE(buf[28:], uint32(byteRate))
	putUint16LE(buf[32:], uint16(blockAlign))
	putUint16LE(buf[34:], 16)
	buf[36] = 'd'
	buf[37] = 'a'
	buf[38] = 't'
	buf[39] = 'a'
	putUint32LE(buf[40:], uint32(dataSize))

	for i, sample := range samples {
		val := max(min(int16(sample*32767.0), 32767), -32768)
		offset := 44 + i*2
		buf[offset] = byte(val)
		buf[offset+1] = byte(val >> 8)
	}

	return buf
}

func putUint16LE(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putUint32LE(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func QwenTTSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("解析TTS请求失败: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: false,
			Error:   "无效的请求格式",
		})
		return
	}

	if req.Text == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: false,
			Error:   "文本内容不能为空",
		})
		return
	}

	entry, exists := ttsCache.Get(req.Text)
	if exists {
		audioBase64 := entry.Wait()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: true,
			Audio:   audioBase64,
		})
		return
	}

	entry, isCreator := ttsCache.GetOrSetPending(req.Text)
	if !isCreator {
		audioBase64 := entry.Wait()
		log.Printf("TTS 缓存等待，文本: %s", req.Text)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: true,
			Audio:   audioBase64,
		})
		return
	}

	samples, err := synthesizeText(req.Text)
	if err != nil {
		log.Printf("TTS 合成失败: %v", err)
		ttsCache.Remove(req.Text)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TTSResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	wavData := encodePCMToWAV(samples, 24000)
	audioBase64 := base64.StdEncoding.EncodeToString(wavData)

	entry.MarkReady(audioBase64)

	log.Printf("TTS 合成成功 [ 文本: %s ] 采样数: %d", req.Text, len(samples))

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TTSResponse{
		Success: true,
		Audio:   audioBase64,
	})
}
