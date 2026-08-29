module LunarSubsystem/BrowserClient

go 1.24.4

require (
	LunarSubsystem/GeneralConfig v0.0.0-00010101000000-000000000000
	LunarSubsystem/LoggerGeneral v0.0.0
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6
)

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/LoggerGeneral => ../logger_general
