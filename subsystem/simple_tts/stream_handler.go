package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 65536,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSStreamRequest struct {
	Text        string `json:"text"`
	RefAudio    string `json:"ref_audio,omitempty"`
	LanguageID  int32  `json:"language_id,omitempty"`
	ChunkFrames int32  `json:"chunk_frames,omitempty"`
}

type WSStreamResponse struct {
	Type         string `json:"type"`
	Audio        string `json:"audio,omitempty"`
	TotalSamples int32  `json:"total_samples,omitempty"`
	SampleRate   int32  `json:"sample_rate,omitempty"`
	IsFinal      bool   `json:"is_final,omitempty"`
	Error        string `json:"error,omitempty"`
}

func TTSStreamHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WSStream] 升级WebSocket失败: %v", err)
		return
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[WSStream] 读取请求失败: %v", err)
		return
	}

	var req WSStreamRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		sendWSResponse(conn, WSStreamResponse{
			Type:  "error",
			Error: "无效的请求格式: " + err.Error(),
		})
		return
	}

	if req.Text == "" {
		sendWSResponse(conn, WSStreamResponse{
			Type:  "error",
			Error: "文本内容不能为空",
		})
		return
	}

	chunkFrames := req.ChunkFrames
	if chunkFrames <= 0 {
		chunkFrames = 50
	}

	ctxID, err := synthesizeTextStreaming(req.Text, req.RefAudio, req.LanguageID, chunkFrames)
	if err != nil {
		sendWSResponse(conn, WSStreamResponse{
			Type:  "error",
			Error: err.Error(),
		})
		return
	}

	streamCtxMapMu.Lock()
	ctx, exists := streamCtxMap[ctxID]
	streamCtxMapMu.Unlock()

	if !exists {
		sendWSResponse(conn, WSStreamResponse{
			Type:  "error",
			Error: "流式上下文未找到",
		})
		return
	}

	var totalSamples int32
	var sampleRate int32
	var sendMu sync.Mutex
	done := false

	for !done {
		select {
		case chunk, ok := <-ctx.ch:
			if !ok {
				done = true
				continue
			}

			sampleRate = chunk.SampleRate
			if chunk.IsFinal {
				sendMu.Lock()
				sendWSResponse(conn, WSStreamResponse{
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
				sendWSResponse(conn, WSStreamResponse{
					Type:         "audio_chunk",
					Audio:        audioBase64,
					TotalSamples: totalSamples,
					SampleRate:   sampleRate,
				})
				sendMu.Unlock()
			}

		case <-ctx.done:
			if ctx.err != nil {
				sendMu.Lock()
				sendWSResponse(conn, WSStreamResponse{
					Type:  "error",
					Error: ctx.err.Error(),
				})
				sendMu.Unlock()
			} else {
				sendMu.Lock()
				sendWSResponse(conn, WSStreamResponse{
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

func sendWSResponse(conn *websocket.Conn, resp WSStreamResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[WSStream] 序列化响应失败: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[WSStream] 发送WebSocket消息失败: %v", err)
	}
}

func float32ToPCM16(samples []float32) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		val := int16(math.Max(-32768, math.Min(32767, float64(s*32767.0))))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(val))
	}
	return buf
}
