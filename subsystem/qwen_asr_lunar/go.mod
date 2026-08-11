module qwen-asr-server

go 1.24.4

replace LunarSubsystem/browser_client => ../browser_client

replace LunarSubsystem/general_config => ../general_config

require LunarSubsystem/browser_client v0.0.0-00010101000000-000000000000

require (
	LunarSubsystem/general_config v0.0.0-00010101000000-000000000000
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
)

require LunarSubsystem/general_logger v0.0.0

replace LunarSubsystem/general_logger => ../general_logger
