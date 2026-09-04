module LunarSubsystem/AutoLTP

go 1.24.4

require (
	LunarSubsystem/GeneralConfig v0.0.0
	LunarSubsystem/ImageProcessor v0.0.0
)

require (
	LunarSubsystem/LoggerGeneral v0.0.0 // indirect
	github.com/aws/aws-sdk-go v1.38.20 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/gen2brain/shm v0.1.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kbinani/screenshot v0.0.0-20250624051815-089614a94018 // indirect
	github.com/u2takey/ffmpeg-go v0.5.0 // indirect
	github.com/u2takey/go-utils v0.3.1 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

require (
	github.com/auuunya/go-element v1.0.1
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8 // indirect
)

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/ImageProcessor => ../image_processor

replace LunarSubsystem/LoggerGeneral => ../logger_general
