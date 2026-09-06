module LunarSubsystem/Kokoro-TTS

go 1.25.0

replace LunarSubsystem/GeneralConfig => ../general_config

replace LunarSubsystem/LoggerGeneral => ../logger_general

require (
	LunarSubsystem/GeneralConfig v0.0.0-00010101000000-000000000000
	LunarSubsystem/LoggerGeneral v0.0.0-00010101000000-000000000000
	github.com/mozillazg/go-pinyin v0.21.0
	github.com/yalue/onnxruntime_go v1.36.0
	github.com/yanyiwu/gojieba v1.4.7
)
