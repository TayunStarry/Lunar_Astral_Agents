package tts

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"qwen3_tts_lunar/module"

	"github.com/gorilla/websocket"
)

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

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: false,
			Error:   "无法读取请求体",
		})
		return
	}
	r.Body.Close()

	var req module.TTSRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		log.Printf("[TTSWrap] 解析TTS请求失败: %v", err)
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

	entry, exists := ttsWrapperCache.Get(req.Text)
	if exists {
		audioBase64 := entry.Wait()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: true,
			Audio:   audioBase64,
		})
		return
	}

	entry, isCreator := ttsWrapperCache.GetOrSetPending(req.Text)
	if !isCreator {
		audioBase64 := entry.Wait()
		log.Printf("[TTSWrap] 缓存命中（等待中），文本: %s", req.Text)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(module.TTSResponse{
			Success: true,
			Audio:   audioBase64,
		})
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	capture := &responseCapture{ResponseWriter: w, statusCode: 200}
	module.TTSHandler(capture, r)

	if capture.statusCode == http.StatusOK {
		var resp module.TTSResponse
		if err := json.Unmarshal(capture.body.Bytes(), &resp); err == nil && resp.Success {
			entry.MarkReady(resp.Audio)
			return
		}
	}

	ttsWrapperCache.Remove(req.Text)
	entry.MarkReady("")
	log.Printf("[TTSWrap] TTS合成失败，文本: %s", req.Text)
}

func TTSStreamHandlerWrapper(w http.ResponseWriter, r *http.Request) {
	conn, err := ttsStreamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[TTSStreamWrap] WebSocket升级失败: %v", err)
		return
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[TTSStreamWrap] 读取请求失败: %v", err)
		return
	}

	var req module.WSStreamRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		sendStreamWrapError(conn, "无效的请求格式: "+err.Error())
		return
	}

	if req.Text == "" {
		sendStreamWrapError(conn, "文本内容不能为空")
		return
	}

	entry, exists := ttsWrapperCache.Get(req.Text)
	if exists {
		audioBase64 := entry.Wait()
		if audioBase64 == "" {
			sendStreamWrapError(conn, "TTS合成失败")
			return
		}
		streamWAVToWebSocket(conn, audioBase64)
		return
	}

	entry, isCreator := ttsWrapperCache.GetOrSetPending(req.Text)
	if !isCreator {
		audioBase64 := entry.Wait()
		log.Printf("[TTSStreamWrap] 缓存命中（等待中），文本: %s", req.Text)
		if audioBase64 == "" {
			sendStreamWrapError(conn, "TTS合成失败")
			return
		}
		streamWAVToWebSocket(conn, audioBase64)
		return
	}

	go func() {
		reqBody, _ := json.Marshal(module.TTSRequest{Text: req.Text})
		httpReq, _ := http.NewRequest("POST", "/tts", bytes.NewReader(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		mockWriter := &ttsMockWriter{statusCode: 200}
		module.TTSHandler(mockWriter, httpReq)

		if mockWriter.statusCode == http.StatusOK {
			var ttsResp module.TTSResponse
			if err := json.Unmarshal(mockWriter.body.Bytes(), &ttsResp); err == nil && ttsResp.Success {
				entry.MarkReady(ttsResp.Audio)
				return
			}
		}
		log.Printf("[TTSStreamWrap] 内部合成失败，文本: %s", req.Text)
		ttsWrapperCache.Remove(req.Text)
		entry.MarkReady("")
	}()

	audioBase64 := entry.Wait()
	if audioBase64 == "" {
		sendStreamWrapError(conn, "TTS合成失败")
		return
	}
	streamWAVToWebSocket(conn, audioBase64)
}

func sendStreamWrapError(conn *websocket.Conn, errMsg string) {
	resp := module.WSStreamResponse{
		Type:  "error",
		Error: errMsg,
	}
	data, _ := json.Marshal(resp)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[TTSStreamWrap] 发送错误消息失败: %v", err)
	}
}

func streamWAVToWebSocket(conn *websocket.Conn, audioBase64 string) {
	wavData, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		sendStreamWrapError(conn, "音频解码失败")
		return
	}

	if len(wavData) < 44 {
		sendStreamWrapError(conn, "无效的WAV数据")
		return
	}

	var dataStart, dataSize int
	sampleRate := int32(binary.LittleEndian.Uint32(wavData[24:28]))
	bitsPerSample := int16(binary.LittleEndian.Uint16(wavData[34:36]))

	if string(wavData[36:40]) == "data" {
		dataSize = int(binary.LittleEndian.Uint32(wavData[40:44]))
		dataStart = 44
	} else {
		pos := 12
		for pos < len(wavData)-8 {
			chunkID := string(wavData[pos : pos+4])
			chunkSize := int(binary.LittleEndian.Uint32(wavData[pos+4 : pos+8]))
			if chunkID == "data" {
				dataSize = chunkSize
				dataStart = pos + 8
				break
			}
			pos += 8 + chunkSize
		}
		if dataStart == 0 {
			sendStreamWrapError(conn, "无法解析WAV数据块")
			return
		}
	}

	if dataSize == 0 || dataStart+dataSize > len(wavData) {
		sendStreamWrapError(conn, "无效的WAV数据大小")
		return
	}

	pcmData := wavData[dataStart : dataStart+dataSize]
	bytesPerSample := int(bitsPerSample) / 8
	numSamples := dataSize / bytesPerSample
	chunkSamples := 1200

	var totalSamples int32
	var chunkIndex int32

	for offset := 0; offset < numSamples; offset += chunkSamples {
		end := offset + chunkSamples
		if end > numSamples {
			end = numSamples
		}
		chunkPCM := pcmData[offset*bytesPerSample : end*bytesPerSample]
		chunkBase64 := base64.StdEncoding.EncodeToString(chunkPCM)

		currentSamples := int32(end - offset)
		totalSamples += currentSamples
		chunkIndex++
		isFinal := end >= numSamples

		resp := module.WSStreamResponse{
			Type:         "audio_chunk",
			Audio:        chunkBase64,
			ChunkIndex:   chunkIndex,
			TotalSamples: totalSamples,
			SampleRate:   sampleRate,
			IsFinal:      isFinal,
		}
		data, _ := json.Marshal(resp)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}

		if isFinal {
			finalResp := module.WSStreamResponse{
				Type:         "final",
				ChunkIndex:   chunkIndex,
				TotalChunks:  chunkIndex,
				TotalSamples: totalSamples,
				SampleRate:   sampleRate,
				IsFinal:      true,
			}
			data, _ := json.Marshal(finalResp)
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}
