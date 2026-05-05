module screenshot

go 1.24.4

require github.com/disintegration/imaging v1.6.2

require (
	github.com/gen2brain/shm v0.1.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	golang.org/x/sys v0.41.0 // indirect
)

require (
	github.com/kbinani/screenshot v0.0.0-20250624051815-089614a94018
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
)

require config v0.0.0

replace config => ../config
