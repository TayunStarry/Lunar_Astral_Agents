package ttsrouter

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	KokoroTTS "CorePersonality/engine/kokoro_tts"
	OmniVoiceTTS "CorePersonality/engine/omnivoice_tts"
	qwentts "CorePersonality/engine/qwen_tts"
	"LunarSubsystem/GeneralConfig"
	logger "LunarSubsystem/LoggerGeneral"
)

// ==== 引擎惰性加载（统一管理三个 TTS 引擎的加载点） ====
//
// 全部引擎均为惰性加载：首次合成请求到达时才加载模型；引擎包内部
// 支持 5 分钟闲置自动卸载（lifecycle.go 守护触发），卸载后由本文件的
// Ensure* 在下一次请求时重新加载。

// localData 以可执行文件目录为基准解析本地数据目录（绝对路径）
func localData() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), *GeneralConfig.LocalDir)
	}
	return *GeneralConfig.LocalDir
}

// resolveAssetPath 解析资源相对路径为绝对路径（以可执行文件目录为基准）；
// 绝对路径与空串原样返回
func resolveAssetPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), p)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return p
}

// EnsureQwen 惰性加载 Qwen TTS 引擎：模型目录固定，参考音频取配置（缺省月华模板音色）
func EnsureQwen() error {
	modelDir := filepath.Join(localData(), "models", qwenModelDirName)
	refAudio := *GeneralConfig.TTSRefAudio
	if refAudio == "" {
		refAudio = filepath.Join(*GeneralConfig.LocalDir, qwenDefaultRefAudio)
	}
	return qwentts.EnsureLoaded(modelDir, resolveAssetPath(refAudio))
}

// EnsureKokoro 惰性加载 Kokoro 引擎：模型目录固定，onnxruntime.dll 随模型目录放置。
// 保留 panic 防护（onnx 回调路径的 panic 会击穿普通错误返回），异常落盘日志。
func EnsureKokoro() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Kokoro 引擎初始化 panic: %v\n%s", r, debug.Stack())
			logger.Error("TTS-Router", "%v", err)
			writeEngineInitError("kokoro_init_error.log", fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
		}
	}()

	modelDir := filepath.Join(localData(), "models", kokoroModelDirName)
	dllPath := filepath.Join(modelDir, kokoroOnnxDllName)
	if _, serr := os.Stat(dllPath); serr == nil {
		KokoroTTS.SetOnnxLibraryPath(dllPath)
	}
	return KokoroTTS.InitEngine(
		modelDir,
		filepath.Join(modelDir, "voices"),
		filepath.Join(modelDir, "espeak"),
	)
}

// EnsureOmniVoice 惰性加载 OmniVoice 引擎：模型目录固定；
// 注入参考音频相对路径的解析基目录（兼容 "./audios/x.wav" 等写法）
func EnsureOmniVoice() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("OmniVoice 引擎初始化 panic: %v\n%s", r, debug.Stack())
			logger.Error("TTS-Router", "%v", err)
			writeEngineInitError("omnivoice_init_error.log", fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
		}
	}()

	modelDir := filepath.Join(localData(), "models", omniModelDirName)
	OmniVoiceTTS.SetRefAudioBaseDirs([]string{localData()})
	return OmniVoiceTTS.InitEngine(modelDir)
}

// writeEngineInitError 将引擎初始化错误写入日志文件（windowsgui 构建无控制台，必须落盘才能排查）
func writeEngineInitError(filename, msg string) {
	logDir := filepath.Join(localData(), "logs")
	_ = os.MkdirAll(logDir, 0755)
	_ = os.WriteFile(filepath.Join(logDir, filename), []byte(msg+"\n"), 0644)
}
