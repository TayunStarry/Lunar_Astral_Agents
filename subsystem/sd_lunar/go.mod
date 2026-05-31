module sd_lunar

go 1.24.4

require (
	browser v0.0.0
	config v0.0.0
	logger v0.0.0
)

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect

replace config => ../config

replace browser => ../browser

replace logger => ../logger
