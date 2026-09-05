module LunarSubsystem/LTP3Keygen

go 1.26

require (
	LunarSubsystem/BrowserClient v0.0.0
	LunarSubsystem/GeneralConfig v0.0.0
	LunarSubsystem/LoggerGeneral v0.0.0
	LunarSubsystem/LunarDecoder v0.0.0
)

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6 // indirect

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/LoggerGeneral => ../logger_general

replace LunarSubsystem/BrowserClient => ../browser_client

replace LunarSubsystem/LunarDecoder => ../lunar_decoder
