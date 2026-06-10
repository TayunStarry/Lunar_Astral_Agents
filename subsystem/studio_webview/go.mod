module studio_webview

go 1.24.4

require browser v0.0.0

require (
	config v0.0.0
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
	logger v0.0.0 // indirect
)

replace config => ../config

replace browser => ../browser

replace logger => ../logger
