package tts

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// 全局TTS缓存实例，容量为5条记录
var ttsCache = NewTTSCache(MaxCacheSize)

// WebSocket升级器，用于流式TTS服务
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 65536,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
