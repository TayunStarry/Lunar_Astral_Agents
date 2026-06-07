module proxy

go 1.24.4

require config v0.0.0

replace config => ../config

require logger v0.0.0

replace logger => ../logger

require browser v0.0.0

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect

replace browser => ../browser
