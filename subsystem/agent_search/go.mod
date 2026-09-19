module LunarSubsystem/AgentSearch

go 1.26

require (
	LunarSubsystem/FileManager v0.0.0-00010101000000-000000000000
	LunarSubsystem/GeneralConfig v0.0.0
	LunarSubsystem/LoggerGeneral v0.0.0
	github.com/chromedp/chromedp v0.16.0
	github.com/shirou/gopsutil/v3 v3.20.10
	golang.org/x/net v0.40.0
)

replace LunarSubsystem/FileManager => ../file_manager

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/LoggerGeneral => ../logger_general

replace LunarSubsystem/MultimodalAnalysis => ../multimodal_analysis

require (
	LunarSubsystem/MultimodalAnalysis v0.0.0 // indirect
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/aws/aws-sdk-go v1.38.20 // indirect
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/gen2brain/shm v0.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kbinani/screenshot v0.0.0-20250624051815-089614a94018 // indirect
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	github.com/mattn/go-sqlite3 v1.14.33 // indirect
	github.com/u2takey/ffmpeg-go v0.5.0 // indirect
	github.com/u2takey/go-utils v0.3.1 // indirect
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
