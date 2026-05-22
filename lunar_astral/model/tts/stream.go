package tts

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
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"unsafe"

	gws "github.com/gorilla/websocket"

	ws "LunarCore/websocket"
)

//export streamPCMCallback
func streamPCMCallback(samples *C.float, nSamples C.int32_t, sampleRate C.int32_t, isFinal C.int, userData unsafe.Pointer) C.int {
	entry := (*streamCacheEntry)(userData)

	if atomic.LoadInt32(&entry.aborted) != 0 {
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

	entry.mu.Lock()
	if chunk.SampleRate != 0 {
		entry.sampleRate = chunk.SampleRate
	}

	for i := len(entry.subscribers) - 1; i >= 0; i-- {
		subCh := entry.subscribers[i]
		select {
		case subCh <- chunk:
		default:
			close(subCh)
			entry.subscribers = append(entry.subscribers[:i], entry.subscribers[i+1:]...)
		}
	}
	entry.mu.Unlock()

	return 0
}

func getOrCreateStreamEntry(text, refAudio string, languageID int32, chunkFrames int32) (*streamCacheEntry, chan StreamPCMChunk, bool) {
	streamCacheMu.Lock()
	defer streamCacheMu.Unlock()

	if entry, exists := streamCacheItems[text]; exists {
		subCh := make(chan StreamPCMChunk, 4)
		entry.mu.Lock()
		entry.subscribers = append(entry.subscribers, subCh)
		entry.mu.Unlock()
		return entry, subCh, false
	}

	entry := &streamCacheEntry{
		text:        text,
		refAudio:    refAudio,
		languageID:  languageID,
		chunkFrames: chunkFrames,
		done:        make(chan struct{}),
	}

	subCh := make(chan StreamPCMChunk, 4)
	entry.subscribers = append(entry.subscribers, subCh)

	streamCacheItems[text] = entry

	return entry, subCh, true
}

func removeStreamEntry(text string) {
	streamCacheMu.Lock()
	defer streamCacheMu.Unlock()

	if entry, ok := streamCacheItems[text]; ok {
		entry.mu.Lock()
		for _, subCh := range entry.subscribers {
			select {
			case <-subCh:
			default:
			}
			close(subCh)
		}
		entry.mu.Unlock()
		delete(streamCacheItems, text)
	}
}

func synthesizeTextStreaming(text, refAudio string, languageID int32, chunkFrames int32, entry *streamCacheEntry) {
	if globalTTS == nil || globalTTS.handle == nil {
		entry.mu.Lock()
		entry.err = fmt.Errorf("TTS 引擎未初始化")
		entry.mu.Unlock()
		close(entry.done)
		return
	}

	if refAudio == "" {
		refAudio = globalTTS.refAudio
	}
	if languageID == 0 {
		languageID = globalTTS.languageID
	}

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	cRefAudio := C.CString(refAudio)
	defer C.free(unsafe.Pointer(cRefAudio))

	var cParams C.Qwen3TtsParams
	C.qwen3_tts_default_params(&cParams)
	cParams.n_threads = 4
	cParams.language_id = C.int32_t(languageID)

	entryPtr := unsafe.Pointer(entry)

	go func() {
		result := C.qwen3_tts_synthesize_streaming(
			globalTTS.handle,
			cText,
			cRefAudio,
			&cParams,
			C.qwen3_tts_stream_callback(C.streamPCMCallback),
			entryPtr,
			C.int32_t(chunkFrames),
		)

		entry.mu.Lock()
		defer entry.mu.Unlock()
		defer close(entry.done)

		if result != 0 {
			errStr := C.GoString(C.qwen3_tts_get_error(globalTTS.handle))
			entry.err = fmt.Errorf("TTS 流式合成失败: %s", errStr)
		}
	}()
}

func float32ToPCM16(samples []float32) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		val := int16(math.Max(-32768, math.Min(32767, float64(s*32767.0))))
		buf[i*2] = byte(val)
		buf[i*2+1] = byte(val >> 8)
	}
	return buf
}

func QwenTTSStreamHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[TTSStream] WebSocket升级失败: %v", err)
		return
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[TTSStream] 读取请求失败: %v", err)
		return
	}

	var req WSStreamRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		sendWSStreamResponse(conn, WSStreamResponse{
			Type:  "error",
			Error: "无效的请求格式: " + err.Error(),
		})
		return
	}

	if req.Text == "" {
		sendWSStreamResponse(conn, WSStreamResponse{
			Type:  "error",
			Error: "文本内容不能为空",
		})
		return
	}

	chunkFrames := req.ChunkFrames
	if chunkFrames <= 0 {
		chunkFrames = 50
	}

	entry, subCh, isCreator := getOrCreateStreamEntry(req.Text, req.RefAudio, req.LanguageID, chunkFrames)

	if isCreator {
		synthesizeTextStreaming(req.Text, req.RefAudio, req.LanguageID, chunkFrames, entry)
		defer removeStreamEntry(req.Text)
	}

	var totalSamples int32
	var sampleRate int32
	var sendMu sync.Mutex
	done := false

	for !done {
		select {
		case chunk, ok := <-subCh:
			if !ok {
				done = true
				continue
			}

			sampleRate = chunk.SampleRate
			if chunk.IsFinal {
				sendMu.Lock()
				sendWSStreamResponse(conn, WSStreamResponse{
					Type:         "final",
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
					IsFinal:      true,
				})
				sendMu.Unlock()
				done = true
				continue
			}

			if len(chunk.Samples) > 0 {
				pcmData := float32ToPCM16(chunk.Samples)
				audioBase64 := base64.StdEncoding.EncodeToString(pcmData)
				totalSamples += int32(len(chunk.Samples))

				sendMu.Lock()
				sendWSStreamResponse(conn, WSStreamResponse{
					Type:         "audio_chunk",
					Audio:        audioBase64,
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
				})
				sendMu.Unlock()
			}

		case <-entry.done:
			entry.mu.Lock()
			streamErr := entry.err
			entry.mu.Unlock()

			if streamErr != nil {
				sendMu.Lock()
				sendWSStreamResponse(conn, WSStreamResponse{
					Type:  "error",
					Error: streamErr.Error(),
				})
				sendMu.Unlock()
			} else {
				sendMu.Lock()
				sendWSStreamResponse(conn, WSStreamResponse{
					Type:         "final",
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
					IsFinal:      true,
				})
				sendMu.Unlock()
			}
			done = true
		}
	}
}

func sendWSStreamResponse(conn *gws.Conn, resp WSStreamResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[TTSStream] 序列化响应失败: %v", err)
		return
	}

	if err := conn.WriteMessage(gws.TextMessage, data); err != nil {
		log.Printf("[TTSStream] 发送WebSocket消息失败: %v", err)
	}
}
