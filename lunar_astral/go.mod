module lunar_astral

go 1.26

require (
	browser v0.0.0
	config v0.0.0
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14
	github.com/gorilla/websocket v1.5.3
	image v0.0.0-00010101000000-000000000000
	logger v0.0.0
	qwen3_tts_lunar v0.0.0
	screenshot v0.0.0
	storage v0.0.0
	websearch v0.0.0
)

require (
	github.com/aws/aws-sdk-go v1.38.20 // indirect
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/chromedp v0.16.0 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/gen2brain/shm v0.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kbinani/screenshot v0.0.0-20250624051815-089614a94018 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	github.com/mattn/go-sqlite3 v1.14.33 // indirect
	github.com/u2takey/ffmpeg-go v0.5.0 // indirect
	github.com/u2takey/go-utils v0.3.1 // indirect
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.25.0 // indirect
)

replace config => ../subsystem/config

replace storage => ../subsystem/storage

replace browser => ../subsystem/browser

replace screenshot => ../subsystem/screenshot

replace qwen3_tts_lunar => ../subsystem/qwen3_tts_lunar

replace logger => ../subsystem/logger

replace image => ../subsystem/image

replace websearch => ../subsystem/websearch
