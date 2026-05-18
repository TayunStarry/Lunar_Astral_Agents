package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWSStreamRequest_UnmarshalJSON(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid request with all fields",
			input:   `{"text":"hello world","ref_audio":"test.wav","language_id":2055,"chunk_frames":50}`,
			wantErr: false,
		},
		{
			name:    "minimal request with text only",
			input:   `{"text":"test"}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req WSStreamRequest
			err := json.Unmarshal([]byte(tc.input), &req)
			if (err != nil) != tc.wantErr {
				t.Errorf("unexpected error state: got %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestWSStreamHandler_InvalidRequest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "non-WebSocket GET request should fail upgrade",
			method:     "GET",
			path:       "/tts/stream",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")

			w := httptest.NewRecorder()
			TTSStreamHandler(w, req)

			resp := w.Result()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestWSStreamHandler_EmptyText(t *testing.T) {
	if globalTTS == nil {
		t.Skip("TTS engine not initialized")
	}

	server := httptest.NewServer(http.HandlerFunc(TTSStreamHandler))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/tts/stream"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer ws.Close()

	req := WSStreamRequest{
		Text: "",
	}
	reqData, _ := json.Marshal(req)
	ws.WriteMessage(websocket.TextMessage, reqData)

	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	var resp WSStreamResponse
	json.Unmarshal(msg, &resp)

	if resp.Type != "error" {
		t.Errorf("expected error response, got type: %s", resp.Type)
	}
	if resp.Error == "" {
		t.Error("expected error message to be non-empty")
	}
}

func TestFloat32ToPCM16(t *testing.T) {
	testCases := []struct {
		name     string
		samples  []float32
		wantLen  int
		wantData []byte
	}{
		{
			name:     "single zero sample",
			samples:  []float32{0.0},
			wantLen:  2,
			wantData: []byte{0x00, 0x00},
		},
		{
			name:     "single max positive sample",
			samples:  []float32{1.0},
			wantLen:  2,
			wantData: []byte{0xFF, 0x7F},
		},
		{
			name:     "single max negative sample",
			samples:  []float32{-1.0},
			wantLen:  2,
			wantData: []byte{0x00, 0x80},
		},
		{
			name:     "multiple samples",
			samples:  []float32{0.0, 0.5, -0.5},
			wantLen:  6,
			wantData: []byte{0x00, 0x00, 0x00, 0x40, 0x00, 0xC0},
		},
		{
			name:     "clipping positive",
			samples:  []float32{2.0},
			wantLen:  2,
			wantData: []byte{0xFF, 0x7F},
		},
		{
			name:     "clipping negative",
			samples:  []float32{-2.0},
			wantLen:  2,
			wantData: []byte{0x00, 0x80},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := float32ToPCM16(tc.samples)
			if len(got) != tc.wantLen {
				t.Errorf("length = %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantData != nil {
				for i := range tc.wantData {
					if got[i] != tc.wantData[i] {
						t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, got[i], tc.wantData[i])
					}
				}
			}
		})
	}
}

func TestStreamPCMChunk_Structure(t *testing.T) {
	chunk := StreamPCMChunk{
		Samples:    []float32{0.1, 0.2, 0.3},
		SampleRate: 24000,
		IsFinal:    false,
	}

	if chunk.SampleRate != 24000 {
		t.Errorf("sample rate = %d, want 24000", chunk.SampleRate)
	}
	if chunk.IsFinal {
		t.Error("expected IsFinal to be false")
	}
	if len(chunk.Samples) != 3 {
		t.Errorf("samples count = %d, want 3", len(chunk.Samples))
	}
}

func TestWSStreamResponse_MarshalJSON(t *testing.T) {
	testCases := []struct {
		name     string
		resp     WSStreamResponse
		contains string
	}{
		{
			name: "audio chunk response",
			resp: WSStreamResponse{
				Type:         "audio_chunk",
				Audio:        "base64data",
				TotalSamples: 24000,
				SampleRate:   24000,
			},
			contains: `"type":"audio_chunk"`,
		},
		{
			name: "error response",
			resp: WSStreamResponse{
				Type:  "error",
				Error: "test error",
			},
			contains: `"type":"error"`,
		},
		{
			name: "final response",
			resp: WSStreamResponse{
				Type:         "final",
				TotalSamples: 48000,
				SampleRate:   24000,
				IsFinal:      true,
			},
			contains: `"type":"final"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.resp)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			if !strings.Contains(string(data), tc.contains) {
				t.Errorf("response does not contain %q: %s", tc.contains, string(data))
			}
		})
	}
}

func TestStreamingContext_Creation(t *testing.T) {
	streamCtxMapMu.Lock()
	streamCtxCounter++
	ctxID := streamCtxCounter
	ctx := &streamingContext{
		ch:   make(chan StreamPCMChunk, 4),
		done: make(chan struct{}),
	}
	streamCtxMap[ctxID] = ctx
	streamCtxMapMu.Unlock()

	streamCtxMapMu.Lock()
	gotCtx, exists := streamCtxMap[ctxID]
	streamCtxMapMu.Unlock()

	if !exists {
		t.Fatal("context should exist")
	}
	if gotCtx.ch == nil {
		t.Error("channel should be initialized")
	}
	if gotCtx.done == nil {
		t.Error("done channel should be initialized")
	}

	streamCtxMapMu.Lock()
	delete(streamCtxMap, ctxID)
	streamCtxMapMu.Unlock()
}

func TestSynthesizeTextStreaming_NilEngine(t *testing.T) {
	originalTTS := globalTTS
	globalTTS = nil

	_, err := synthesizeTextStreaming("test", "", 2055, 50)
	if err == nil {
		t.Error("expected error when TTS engine is nil")
	}

	globalTTS = originalTTS
}

func TestEndpoint_Registration(t *testing.T) {
	found := false
	for _, ep := range endpoints {
		if ep.Path == "/tts/stream" {
			found = true
			if ep.Method != "GET" {
				t.Errorf("expected method GET, got %s", ep.Method)
			}
			if ep.Handler == nil {
				t.Error("handler should not be nil")
			}
			break
		}
	}

	if !found {
		t.Error("/tts/stream endpoint not registered")
	}
}
