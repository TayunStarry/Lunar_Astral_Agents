module crystal_astral

go 1.24.4

require config v0.0.0

replace config => ../config

require storage v0.0.0

replace storage => ../storage

require browser v0.0.0

replace browser => ../browser

require github.com/mattn/go-sqlite3 v1.14.33 // indirect

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
