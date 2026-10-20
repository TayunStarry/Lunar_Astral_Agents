module LunarSubsystem/BrowserClient

go 1.24.4

require (
	LunarSubsystem/GeneralConfig v0.0.0-00010101000000-000000000000
	LunarSubsystem/LoggerGeneral v0.0.0
	LunarSubsystem/WindowCapture v0.0.0
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6
)

require (
	github.com/gen2brain/shm v0.1.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/kbinani/screenshot v0.0.0-20250624051815-089614a94018 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	golang.org/x/sys v0.24.0 // indirect
)

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/LoggerGeneral => ../logger_general

replace LunarSubsystem/WindowCapture => ../window_capture
