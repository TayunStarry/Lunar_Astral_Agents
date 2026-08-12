module LunarSubsystem/qwen_asr_lunar

go 1.24.4

replace LunarSubsystem/BrowserClient => ../browser_client

replace LunarSubsystem/GeneralConfig => ../general_config

require LunarSubsystem/BrowserClient v0.0.0

require (
	LunarSubsystem/GeneralConfig v0.0.0
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect
)

require LunarSubsystem/LoggerGeneral v0.0.0

replace LunarSubsystem/LoggerGeneral => ../logger_general
