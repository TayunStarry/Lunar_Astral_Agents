package tts

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"logger"
	"math"
	"net/http"
	"qwen3_tts_lunar/module"
	"sync"

	"github.com/gorilla/websocket"
)

// TTSHandlerWrapper TTS语音合成服务包装器（POST /tts）
// 集成LRU缓存机制，支持disable_cache参数控制
// 使用singleflight模式防止高并发下的缓存击穿
func TTSHandlerWrapper(w http.ResponseWriter, r *http.Request) {
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

	var req module.TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("TTS-Cache", "解析TTS请求失败: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: false,
			Error:   "无效的请求格式",
		})
		return
	}

	if req.Text == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: false,
			Error:   "文本内容不能为空",
		})
		return
	}

	// 构建缓存键（基于所有影响输出的请求参数）
	cacheKey := buildCacheKey(&req)

	// 如果未禁用缓存，尝试从缓存获取
	if !req.DisableCache {
		if audioData, hit := ttsCache.Get(cacheKey); hit {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(module.TTSResponse{
				Success: true,
				Audio:   audioData,
			})
			return
		}
	}

	// 单飞模式：防止并发重复合成（缓存击穿保护）
	inflight, exists := ttsCache.GetOrCreateInflight(cacheKey)
	if exists {
		logger.SubInfo("TTS-Cache", "单飞", "等待已有合成完成: key=%s", cacheKey[:16])
		audioData, err := inflight.Wait()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(module.TTSResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: true,
			Audio:   audioData,
		})
		return
	}
	defer ttsCache.RemoveInflight(cacheKey)

	// 执行实际的TTS合成
	audioData, err := doSynthesize(req)
	if err != nil {
		inflight.Complete("", err)
		logger.Error("TTS-Cache", "合成失败: [%s] %v", req.Text, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// 无论disable_cache是否为true，都更新缓存
	// 当disable_cache=true时，强制生成新数据并更新缓存
	ttsCache.Set(cacheKey, audioData)
	inflight.Complete(audioData, nil)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(module.TTSResponse{
		Success: true,
		Audio:   audioData,
	})
}

// TTSStreamHandlerWrapper TTS流式合成服务包装器（GET /tts/stream）
// 通过WebSocket进行流式音频传输，集成缓存机制
func TTSStreamHandlerWrapper(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("TTS-Cache", "升级WebSocket失败: %v", err)
		return
	}
	defer conn.Close()

	// 读取客户端发送的请求参数
	_, msg, err := conn.ReadMessage()
	if err != nil {
		logger.Error("TTS-Cache", "读取WebSocket请求失败: %v", err)
		return
	}

	var req module.WSStreamRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		sendWSError(conn, "无效的请求格式: "+err.Error())
		return
	}

	if req.Text == "" {
		sendWSError(conn, "文本内容不能为空")
		return
	}

	// 构建缓存键
	cacheKey := buildStreamCacheKey(&req)

	// 如果未禁用缓存，尝试从缓存获取完整音频
	if !req.DisableCache {
		if audioData, hit := ttsCache.Get(cacheKey); hit {
			// 缓存命中：将完整WAV音频作为单个PCM块发送
			sendCachedAudioAsStream(conn, audioData)
			return
		}
	}

	// 缓存未命中：执行流式合成
	chunkFrames := req.ChunkFrames
	if chunkFrames <= 0 {
		chunkFrames = 50
	}

	ctxID, err := module.SynthesizeTextStreaming(
		req.Text, req.RefAudio, req.LanguageID, chunkFrames,
		req.Temperature, req.TopK, req.TopP, req.MaxTokens,
		req.RepetitionPenalty, req.Threads,
	)
	if err != nil {
		sendWSError(conn, err.Error())
		return
	}

	// 获取流式上下文，保持引用以便检查错误
	ctx, exists := module.GetStreamContext(ctxID)
	if !exists {
		sendWSError(conn, "流式上下文未找到")
		return
	}

	// 流式传输音频块，同时累积所有样本用于缓存
	var allSamples []float32
	var totalSamples int32
	var sampleRate int32
	var chunkIndex int32
	var sendMu sync.Mutex
	streamDone := false

	for !streamDone {
		select {
		case chunk, ok := <-ctx.Ch:
			if !ok {
				streamDone = true
				continue
			}

			sampleRate = chunk.SampleRate
			if chunk.IsFinal {
				chunkIndex++
				if len(chunk.Samples) > 0 {
					allSamples = append(allSamples, chunk.Samples...)
					pcmData := Float32ToPCM16(chunk.Samples)
					audioBase64 := base64.StdEncoding.EncodeToString(pcmData)
					totalSamples += int32(len(chunk.Samples))
					sendMu.Lock()
					sendWSResponse(conn, module.WSStreamResponse{
						Type:         "audio_chunk",
						Audio:        audioBase64,
						ChunkIndex:   chunkIndex,
						TotalSamples: totalSamples,
						SampleRate:   sampleRate,
						IsFinal:      true,
					})
					sendMu.Unlock()
				}

				sendMu.Lock()
				sendWSResponse(conn, module.WSStreamResponse{
					Type:         "final",
					ChunkIndex:   chunkIndex,
					TotalChunks:  chunkIndex,
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
					IsFinal:      true,
				})
				sendMu.Unlock()

				// 缓存完整的合成结果
				if len(allSamples) > 0 {
					wavData := module.EncodePCMToWAV(allSamples, int(sampleRate))
					cachedAudio := base64.StdEncoding.EncodeToString(wavData)
					ttsCache.Set(cacheKey, cachedAudio)
				}

				streamDone = true
				continue
			}

			if len(chunk.Samples) > 0 {
				allSamples = append(allSamples, chunk.Samples...)
				chunkIndex++
				pcmData := Float32ToPCM16(chunk.Samples)
				audioBase64 := base64.StdEncoding.EncodeToString(pcmData)
				totalSamples += int32(len(chunk.Samples))

				sendMu.Lock()
				sendWSResponse(conn, module.WSStreamResponse{
					Type:         "audio_chunk",
					Audio:        audioBase64,
					ChunkIndex:   chunkIndex,
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
				})
				sendMu.Unlock()
			}

		case <-ctx.Done:
			if ctx.Err != nil {
				sendMu.Lock()
				sendWSResponse(conn, module.WSStreamResponse{
					Type:  "error",
					Error: ctx.Err.Error(),
				})
				sendMu.Unlock()
			} else {
				sendMu.Lock()
				sendWSResponse(conn, module.WSStreamResponse{
					Type:         "final",
					ChunkIndex:   chunkIndex,
					TotalChunks:  chunkIndex,
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
					IsFinal:      true,
				})
				sendMu.Unlock()
			}
			streamDone = true
		}
	}
}

// buildCacheKey 根据TTS请求参数构建缓存键
func buildCacheKey(req *module.TTSRequest) string {
	return ComputeCacheKey(map[string]interface{}{
		"text":               req.Text,
		"ref_audio":          req.RefAudio,
		"language_id":        req.LanguageID,
		"temperature":        req.Temperature,
		"top_k":              req.TopK,
		"top_p":              req.TopP,
		"max_tokens":         req.MaxTokens,
		"repetition_penalty": req.RepetitionPenalty,
		"threads":            req.Threads,
	})
}

// buildStreamCacheKey 根据流式请求参数构建缓存键
func buildStreamCacheKey(req *module.WSStreamRequest) string {
	return ComputeCacheKey(map[string]interface{}{
		"text":               req.Text,
		"ref_audio":          req.RefAudio,
		"language_id":        req.LanguageID,
		"temperature":        req.Temperature,
		"top_k":              req.TopK,
		"top_p":              req.TopP,
		"max_tokens":         req.MaxTokens,
		"repetition_penalty": req.RepetitionPenalty,
		"threads":            req.Threads,
	})
}

// doSynthesize 执行实际的TTS语音合成，返回base64编码的WAV音频数据
func doSynthesize(req module.TTSRequest) (string, error) {
	samples, err := module.SynthesizeText(
		req.Text, req.RefAudio, req.LanguageID,
		req.Temperature, req.TopK, req.TopP,
		req.MaxTokens, req.RepetitionPenalty, req.Threads,
	)
	if err != nil {
		return "", err
	}

	wavData := module.EncodePCMToWAV(samples, 24000)
	audioBase64 := base64.StdEncoding.EncodeToString(wavData)
	logger.Info("TTS-Cache", "合成完成: [%s] 采样数: %d", req.Text, len(samples))
	return audioBase64, nil
}

// sendCachedAudioAsStream 将缓存的WAV音频作为流式块发送给客户端
func sendCachedAudioAsStream(conn *websocket.Conn, wavBase64 string) {
	wavData, err := base64.StdEncoding.DecodeString(wavBase64)
	if err != nil {
		sendWSError(conn, "缓存音频解码失败: "+err.Error())
		return
	}

	// 跳过WAV头部（44字节），提取PCM数据
	if len(wavData) < 44 {
		sendWSError(conn, "缓存音频数据无效")
		return
	}
	pcmData := wavData[44:]

	// 将PCM数据转换为float32样本
	sampleCount := len(pcmData) / 2
	samples := make([]float32, sampleCount)
	for i := 0; i < sampleCount; i++ {
		val := int16(binary.LittleEndian.Uint16(pcmData[i*2:]))
		samples[i] = float32(val) / 32767.0
	}

	// 以PCM16格式发送整个音频
	sendChunk := Float32ToPCM16(samples)
	audioBase64 := base64.StdEncoding.EncodeToString(sendChunk)

	sendWSResponse(conn, module.WSStreamResponse{
		Type:         "audio_chunk",
		Audio:        audioBase64,
		ChunkIndex:   1,
		TotalSamples: int32(sampleCount),
		SampleRate:   24000,
		IsFinal:      true,
	})

	sendWSResponse(conn, module.WSStreamResponse{
		Type:         "final",
		ChunkIndex:   1,
		TotalChunks:  1,
		TotalSamples: int32(sampleCount),
		SampleRate:   24000,
		IsFinal:      true,
	})
}

// sendWSResponse 发送WebSocket JSON响应
func sendWSResponse(conn *websocket.Conn, resp module.WSStreamResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		logger.Error("TTS-Cache", "序列化WS响应失败: %v", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		logger.Error("TTS-Cache", "发送WebSocket消息失败: %v", err)
	}
}

// sendWSError 发送WebSocket错误响应
func sendWSError(conn *websocket.Conn, errMsg string) {
	sendWSResponse(conn, module.WSStreamResponse{
		Type:  "error",
		Error: errMsg,
	})
}

// Float32ToPCM16 将float32音频样本转换为PCM16字节数组
func Float32ToPCM16(samples []float32) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		val := int16(math.Max(-32768, math.Min(32767, float64(s*32767.0))))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(val))
	}
	return buf
}
