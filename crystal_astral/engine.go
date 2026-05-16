package main

/*
#cgo LDFLAGS: -L"D:/TTS/qwen3-tts.cpp-main/build" -L"D:/TTS/qwen3-tts.cpp-main/ggml/build/src" "D:/TTS/qwen3-tts.cpp-main/build/libqwen3tts.dll.a" -lqwen3_tts -ltts_transformer -ltext_tokenizer -laudio_tokenizer_encoder -laudio_tokenizer_decoder "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-base.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-cpu.a" -lstdc++ -lpthread
#cgo CFLAGS: -I"D:/TTS/qwen3-tts.cpp-main/src" -I"D:/TTS/qwen3-tts.cpp-main/ggml/include"
#include <stdlib.h>
#include "qwen3tts_c_api.h"
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

type TTSEngine struct {
	handle     *C.Qwen3Tts
	modelDir   string
	refAudio   string
	mu         sync.Mutex
	languageID int32
}

var (
	globalTTS *TTSEngine
	ttsOnce   sync.Once
)

const maxCacheSize = 5

type cacheEntry struct {
	audio string
	ready chan struct{}
}

type TTSCache struct {
	mu    sync.Mutex
	items map[string]*cacheEntry
	order []string
}

var ttsCache = &TTSCache{
	items: make(map[string]*cacheEntry),
	order: make([]string, 0, maxCacheSize),
}

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

type TTSRequest struct {
	Text       string `json:"text"`
	RefAudio   string `json:"ref_audio,omitempty"`
	LanguageID int32  `json:"language_id,omitempty"`
}

type TTSResponse struct {
	Success bool   `json:"success"`
	Audio   string `json:"audio,omitempty"`
	Error   string `json:"error,omitempty"`
}

func initTTSEngine(modelDir, refAudio string) {
	ttsOnce.Do(func() {
		cModelDir := C.CString(modelDir)
		defer C.free(unsafe.Pointer(cModelDir))

		handle := C.qwen3_tts_create(cModelDir, 4)
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

		log.Printf("Qwen TTS 引擎初始化成功")
	})
}

func synthesizeText(text, refAudio string, languageID int32) ([]float32, error) {
	if globalTTS == nil || globalTTS.handle == nil {
		return nil, fmt.Errorf("TTS 引擎未初始化")
	}

	if refAudio == "" {
		refAudio = globalTTS.refAudio
	}
	if languageID == 0 {
		languageID = globalTTS.languageID
	}

	globalTTS.mu.Lock()
	defer globalTTS.mu.Unlock()

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cRefAudio := C.CString(refAudio)
	defer C.free(unsafe.Pointer(cRefAudio))

	var cParams C.Qwen3TtsParams
	C.qwen3_tts_default_params(&cParams)
	cParams.n_threads = 4
	cParams.language_id = C.int32_t(languageID)

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
		val := int16(max(-32768, min(32767, int32(sample*32767.0))))
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

func TTSHandler(w http.ResponseWriter, r *http.Request) {
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

	samples, err := synthesizeText(req.Text, req.RefAudio, req.LanguageID)
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

	log.Printf("TTS 合成成功，文本: %s，采样数: %d", req.Text, len(samples))

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TTSResponse{
		Success: true,
		Audio:   audioBase64,
	})
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(32 << 20)

	file, header, err := r.FormFile("audio")
	if err != nil {
		log.Printf("解析上传文件失败: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "无法获取上传文件",
		})
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".wav"
	}

	uploadDir := "./uploads"
	os.MkdirAll(uploadDir, 0755)

	tempPath := filepath.Join(uploadDir, "ref_"+fmt.Sprintf("%d", int(os.Getpid()))+ext)

	outFile, err := os.Create(tempPath)
	if err != nil {
		log.Printf("创建文件失败: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "保存文件失败",
		})
		return
	}
	defer outFile.Close()

	buf := make([]byte, 32*1024)
	for {
		n, _ := file.Read(buf)
		if n == 0 {
			break
		}
		outFile.Write(buf[:n])
	}

	absPath, _ := filepath.Abs(tempPath)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"path":    absPath,
		"name":    header.Filename,
	})
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"service": "simple-tts",
		"port":    36365,
	})
}
