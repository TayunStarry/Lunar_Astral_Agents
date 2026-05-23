module qwen3_tts_lunar

go 1.25.0

replace browser => ../browser

replace config => ../config

require browser v0.0.0

require (
	config v0.0.0-00010101000000-000000000000
	github.com/gorilla/websocket v1.5.3
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
)
