package handlers

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var streamTTSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 65536,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	streamTTSMu       sync.Mutex
	streamTTSBusy     bool
	streamTTSLastReq  string
	streamTTSLastResp *StreamTTSResponse
	streamTTSTimeout  = 30 * time.Second
)

type StreamTTSRequest struct {
	Text        string `json:"text"`
	ChunkFrames int    `json:"chunk_frames,omitempty"`
}

type StreamTTSResponse struct {
	Type         string `json:"type"`
	Audio        string `json:"audio,omitempty"`
	TotalSamples int    `json:"total_samples,omitempty"`
	SampleRate   int    `json:"sample_rate,omitempty"`
	IsFinal      bool   `json:"is_final,omitempty"`
	Error        string `json:"error,omitempty"`
}

func QwenTTSStreamHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := streamTTSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[StreamTTS] WebSocket升级失败: %v", err)
		return
	}
	defer conn.Close()

	var req StreamTTSRequest
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[StreamTTS] 读取请求失败: %v", err)
		return
	}

	if err := json.Unmarshal(msg, &req); err != nil {
		sendStreamTTSResponse(conn, StreamTTSResponse{
			Type:  "error",
			Error: "无效的请求格式: " + err.Error(),
		})
		return
	}

	if req.Text == "" {
		sendStreamTTSResponse(conn, StreamTTSResponse{
			Type:  "error",
			Error: "文本内容不能为空",
		})
		return
	}

	streamTTSMu.Lock()
	if streamTTSBusy {
		if req.Text == streamTTSLastReq && streamTTSLastResp != nil {
			log.Printf("[StreamTTS] 重复请求，返回缓存结果: %s", req.Text)
			sendStreamTTSResponse(conn, *streamTTSLastResp)
			streamTTSMu.Unlock()
			return
		}
		streamTTSMu.Unlock()
		sendStreamTTSResponse(conn, StreamTTSResponse{
			Type:  "error",
			Error: "TTS服务繁忙，请稍候",
		})
		return
	}
	streamTTSBusy = true
	streamTTSMu.Unlock()

	defer func() {
		streamTTSMu.Lock()
		streamTTSBusy = false
		streamTTSMu.Unlock()
	}()

	chunkFrames := req.ChunkFrames
	if chunkFrames <= 0 {
		chunkFrames = 50
	}

	samples, err := synthesizeText(req.Text)
	if err != nil {
		log.Printf("[StreamTTS] 合成失败: %v", err)
		sendStreamTTSResponse(conn, StreamTTSResponse{
			Type:  "error",
			Error: err.Error(),
		})
		return
	}

	sampleRate := 24000
	totalSamples := 0
	chunkSize := chunkFrames * (sampleRate / 100)
	if chunkSize <= 0 {
		chunkSize = 1200
	}

	var allSamples []float32
	for i := 0; i < len(samples); i += chunkSize {
		end := i + chunkSize
		if end > len(samples) {
			end = len(samples)
		}

		chunk := samples[i:end]
		pcmData := float32ToPCM16(chunk)
		audioBase64 := base64.StdEncoding.EncodeToString(pcmData)
		totalSamples += len(chunk)
		allSamples = append(allSamples, chunk...)

		sendStreamTTSResponse(conn, StreamTTSResponse{
			Type:         "audio_chunk",
			Audio:        audioBase64,
			TotalSamples: totalSamples,
			SampleRate:   sampleRate,
		})
	}

	wavData := encodePCMToWAV(allSamples, sampleRate)
	finalAudioBase64 := base64.StdEncoding.EncodeToString(wavData)

	finalResp := StreamTTSResponse{
		Type:         "final",
		Audio:        finalAudioBase64,
		TotalSamples: totalSamples,
		SampleRate:   sampleRate,
		IsFinal:      true,
	}

	streamTTSMu.Lock()
	streamTTSLastReq = req.Text
	streamTTSLastResp = &finalResp
	streamTTSMu.Unlock()

	sendStreamTTSResponse(conn, finalResp)
}

func sendStreamTTSResponse(conn *websocket.Conn, resp StreamTTSResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[StreamTTS] 序列化响应失败: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[StreamTTS] 发送WebSocket消息失败: %v", err)
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
