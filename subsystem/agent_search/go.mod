module LunarSubsystem/AgentSearch

go 1.26

require (
	LunarSubsystem/FileManager v0.0.0-00010101000000-000000000000
	LunarSubsystem/GeneralConfig v0.0.0
	github.com/chromedp/chromedp v0.16.0
	github.com/shirou/gopsutil/v3 v3.20.10
	golang.org/x/net v0.40.0
)

replace LunarSubsystem/FileManager => ../file_manager

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/LoggerGeneral => ../logger_general

require (
	LunarSubsystem/LoggerGeneral v0.0.0 // indirect
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/chromedp/cdproto v0.0.0-20260714215040-dc233986426f // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.33 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
