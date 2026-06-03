module gguf_metadata_viewer

go 1.24.4

replace (
	browser => ../browser
	config => ../config
	logger => ../logger
)

require (
	browser v0.0.0-00010101000000-000000000000
	config v0.0.0-00010101000000-000000000000
	logger v0.0.0
)

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
