module crystal_astral

go 1.24.4

require config v0.0.0

replace config => ../config

require storage v0.0.0

replace storage => ../storage

require browser v0.0.0

replace browser => ../browser

require screenshot v0.0.0

replace screenshot => ../screenshot

require github.com/mattn/go-sqlite3 v1.14.33 // indirect

require (
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/gen2brain/shm v0.1.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/kbinani/screenshot v0.0.0-20250624051815-089614a94018 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
	golang.org/x/sys v0.41.0 // indirect
)
