module LunarSubsystem/LunarGoja

go 1.26

require (
	LunarSubsystem/LoggerGeneral v0.0.0
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14
	github.com/gorilla/websocket v1.5.3
)

require (
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	golang.org/x/text v0.16.0 // indirect
)

replace LunarSubsystem/LoggerGeneral => ../logger_general
