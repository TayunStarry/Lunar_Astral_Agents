module qwen3_tts_lunar

go 1.25.0

replace config => ../config

require (
	config v0.0.0-00010101000000-000000000000
	github.com/gorilla/websocket v1.5.3
)

require logger v0.0.0

replace logger => ../logger
