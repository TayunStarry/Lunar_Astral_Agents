package main

/*
#cgo LDFLAGS: -L"D:/TTS/qwen3-tts.cpp-main/build" -L"D:/TTS/qwen3-tts.cpp-main/ggml/build/src" "D:/TTS/qwen3-tts.cpp-main/build/libqwen3tts.dll.a" -lqwen3_tts -ltts_transformer -ltext_tokenizer -laudio_tokenizer_encoder -laudio_tokenizer_decoder "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-base.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-cpu.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-vulkan/ggml-vulkan.a" "C:/VulkanSDK/1.4.350.0/Lib/libvulkan.dll.a" -lstdc++ -lpthread
#cgo CFLAGS: -I"D:/TTS/qwen3-tts.cpp-main/src" -I"D:/TTS/qwen3-tts.cpp-main/ggml/include"
#include <stdlib.h>
#include "qwen3tts_c_api.h"

//export streamPCMCallback
extern int streamPCMCallback(float* samples, int32_t n_samples, int32_t sample_rate, int is_final, void* user_data);
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
	"runtime"
	"sync"
	"sync/atomic"
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

type speakerEmbedCache struct {
	mu         sync.Mutex
	embeddings map[string][]float32 // refAudio path -> speaker embedding
	cacheDir   string
}

var embedCache = &speakerEmbedCache{
	embeddings: make(map[string][]float32),
	cacheDir:   "./local_data/embed_cache",
}

func (c *speakerEmbedCache) init() {
	os.MkdirAll(c.cacheDir, 0755)
	c.loadFromDisk()
}

func (c *speakerEmbedCache) loadFromDisk() {
	c.mu.Lock()
	defer c.mu.Unlock()

	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		log.Printf("[EmbedCache] 加载磁盘缓存失败: %v", err)
		return
	}

	loaded := 0
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".bin" {
			continue
		}

		filePath := filepath.Join(c.cacheDir, f.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("[EmbedCache] 读取 %s 失败: %v", f.Name(), err)
			continue
		}

		if len(data) < 8 || len(data)%4 != 0 {
			log.Printf("[EmbedCache] 跳过无效文件: %s", f.Name())
			continue
		}

		audioPathLen := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		if 4+audioPathLen+4 > len(data) {
			log.Printf("[EmbedCache] 跳过损坏文件: %s", f.Name())
			continue
		}

		audioPath := string(data[4 : 4+audioPathLen])
		embedSize := int(data[4+audioPathLen])<<24 | int(data[4+audioPathLen+1])<<16 |
			int(data[4+audioPathLen+2])<<8 | int(data[4+audioPathLen+3])

		embedData := data[4+audioPathLen+4:]
		if len(embedData) != embedSize*4 {
			log.Printf("[EmbedCache] 跳过大小不匹配文件: %s", f.Name())
			continue
		}

		embedding := make([]float32, embedSize)
		for i := 0; i < embedSize; i++ {
			bits := uint32(embedData[i*4])<<24 | uint32(embedData[i*4+1])<<16 |
				uint32(embedData[i*4+2])<<8 | uint32(embedData[i*4+3])
			embedding[i] = *(*float32)(unsafe.Pointer(&bits))
		}

		c.embeddings[audioPath] = embedding
		loaded++
	}

	if loaded > 0 {
		log.Printf("[EmbedCache] 从磁盘加载 %d 个 embedding", loaded)
	}
}

func (c *speakerEmbedCache) saveToDisk(audioPath string, embedding []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := fmt.Sprintf("%x", []byte(audioPath))
	if len(hash) > 16 {
		hash = hash[:16]
	}
	fileName := hash + ".bin"
	filePath := filepath.Join(c.cacheDir, fileName)

	audioPathBytes := []byte(audioPath)
	buf := make([]byte, 4+len(audioPathBytes)+4+len(embedding)*4)

	buf[0] = byte(len(audioPathBytes) >> 24)
	buf[1] = byte(len(audioPathBytes) >> 16)
	buf[2] = byte(len(audioPathBytes) >> 8)
	buf[3] = byte(len(audioPathBytes))
	copy(buf[4:], audioPathBytes)

	off := 4 + len(audioPathBytes)
	buf[off] = byte(len(embedding) >> 24)
	buf[off+1] = byte(len(embedding) >> 16)
	buf[off+2] = byte(len(embedding) >> 8)
	buf[off+3] = byte(len(embedding))

	off += 4
	for _, v := range embedding {
		bits := *(*uint32)(unsafe.Pointer(&v))
		buf[off] = byte(bits >> 24)
		buf[off+1] = byte(bits >> 16)
		buf[off+2] = byte(bits >> 8)
		buf[off+3] = byte(bits)
		off += 4
	}

	if err := os.WriteFile(filePath, buf, 0644); err != nil {
		log.Printf("[EmbedCache] 保存 %s 失败: %v", fileName, err)
	}
}

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

		embedCache.init()

		log.Printf("Qwen TTS 引擎初始化成功")

		//go warmupTTSEngine(refAudio)
	})
}

func warmupTTSEngine(refAudio string) {
	log.Println("[Warmup] 开始预热TTS引擎，加载所有模型到内存...")

	dummyText := "你好"
	samples, err := synthesizeText(dummyText, refAudio, globalTTS.languageID)
	if err != nil {
		log.Printf("[Warmup] 预热合成失败（可忽略）: %v", err)
		return
	}

	if len(samples) > 0 {
		log.Printf("[Warmup] 预热成功，生成 %d 个采样点，所有模型已加载完成", len(samples))
	} else {
		log.Println("[Warmup] 预热完成")
	}
}

func getOrExtractEmbedding(refAudio string) ([]float32, error) {
	embedCache.mu.Lock()
	if cached, ok := embedCache.embeddings[refAudio]; ok {
		embedCache.mu.Unlock()
		return cached, nil
	}
	embedCache.mu.Unlock()

	cRefAudio := C.CString(refAudio)
	defer C.free(unsafe.Pointer(cRefAudio))

	const maxEmbedSize = 2048
	embedBuf := make([]float32, maxEmbedSize)

	embedSize := C.qwen3_tts_extract_embedding_file(
		globalTTS.handle,
		cRefAudio,
		(*C.float)(unsafe.Pointer(&embedBuf[0])),
		C.int32_t(maxEmbedSize),
	)

	if embedSize <= 0 {
		errStr := C.GoString(C.qwen3_tts_get_error(globalTTS.handle))
		return nil, fmt.Errorf("提取 speaker embedding失败: %s", errStr)
	}

	embedding := make([]float32, embedSize)
	copy(embedding, embedBuf[:embedSize])

	embedCache.mu.Lock()
	embedCache.embeddings[refAudio] = embedding
	embedCache.mu.Unlock()

	embedCache.saveToDisk(refAudio, embedding)

	log.Printf("[EmbedCache] 已缓存参考音频 %s 的 embedding (size=%d)", refAudio, embedSize)
	return embedding, nil
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

	embedding, err := getOrExtractEmbedding(refAudio)
	if err != nil {
		return nil, err
	}

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	var cParams C.Qwen3TtsParams
	C.qwen3_tts_default_params(&cParams)
	cParams.n_threads = C.int32_t(max(1, runtime.NumCPU()-1))
	cParams.language_id = C.int32_t(languageID)

	result := C.qwen3_tts_synthesize_with_embedding(
		globalTTS.handle,
		cText,
		(*C.float)(unsafe.Pointer(&embedding[0])),
		C.int32_t(len(embedding)),
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

	uploadDir := "./local_data/audios"
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

type StreamPCMChunk struct {
	Samples    []float32
	SampleRate int32
	IsFinal    bool
}

type streamingContext struct {
	ch    chan StreamPCMChunk
	done  chan struct{}
	err   error
	abort int32
}

var streamCtxMap = make(map[int32]*streamingContext)
var streamCtxCounter int32

//export streamPCMCallback
func streamPCMCallback(samples *C.float, nSamples C.int32_t, sampleRate C.int32_t, isFinal C.int, userData unsafe.Pointer) C.int {
	ctxID := *(*C.int32_t)(userData)
	ctxIDInt := int32(ctxID)

	streamCtxMapMu.Lock()
	ctx, exists := streamCtxMap[ctxIDInt]
	streamCtxMapMu.Unlock()

	if !exists {
		return 1
	}

	if atomic.LoadInt32(&ctx.abort) != 0 {
		return 1
	}

	n := int(nSamples)
	chunk := StreamPCMChunk{
		SampleRate: int32(sampleRate),
		IsFinal:    isFinal != 0,
	}

	if n > 0 {
		sampleSlice := (*[1 << 30]C.float)(unsafe.Pointer(samples))[:n:n]
		chunk.Samples = make([]float32, n)
		for i := 0; i < n; i++ {
			chunk.Samples[i] = float32(sampleSlice[i])
		}
	}

	select {
	case ctx.ch <- chunk:
		return 0
	case <-ctx.done:
		atomic.StoreInt32(&ctx.abort, 1)
		return 1
	}
}

var streamCtxMapMu sync.Mutex

func synthesizeTextStreaming(text, refAudio string, languageID int32, chunkFrames int32) (int32, error) {
	if globalTTS == nil || globalTTS.handle == nil {
		return 0, fmt.Errorf("TTS 引擎未初始化")
	}

	if refAudio == "" {
		refAudio = globalTTS.refAudio
	}
	if languageID == 0 {
		languageID = globalTTS.languageID
	}

	streamCtxMapMu.Lock()
	streamCtxCounter++
	ctxID := streamCtxCounter
	ctx := &streamingContext{
		ch:   make(chan StreamPCMChunk, 4),
		done: make(chan struct{}),
	}
	streamCtxMap[ctxID] = ctx
	streamCtxMapMu.Unlock()

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	cRefAudio := C.CString(refAudio)
	defer C.free(unsafe.Pointer(cRefAudio))

	var cParams C.Qwen3TtsParams
	C.qwen3_tts_default_params(&cParams)
	cParams.n_threads = C.int32_t(max(1, runtime.NumCPU()-1))
	cParams.language_id = C.int32_t(languageID)

	cCtxID := C.int32_t(ctxID)
	cChunkFrames := C.int32_t(chunkFrames)

	go func() {
		result := C.qwen3_tts_synthesize_streaming(
			globalTTS.handle,
			cText,
			cRefAudio,
			&cParams,
			C.qwen3_tts_stream_callback(C.streamPCMCallback),
			unsafe.Pointer(&cCtxID),
			cChunkFrames,
		)

		streamCtxMapMu.Lock()
		defer streamCtxMapMu.Unlock()
		delete(streamCtxMap, ctxID)

		if result != 0 {
			errStr := C.GoString(C.qwen3_tts_get_error(globalTTS.handle))
			ctx.err = fmt.Errorf("TTS 流式合成失败: %s", errStr)
		}
		close(ctx.done)
	}()

	return ctxID, nil
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
