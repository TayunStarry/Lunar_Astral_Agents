package main

import (
	"CrystalAstral/engine/OmniVoiceTTS"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
)

// omniInitOnce 引擎懒加载一次性保护（失败后需重启琉璃才能重试）
var omniInitOnce sync.Once

// omniInitErr 引擎初始化错误（供后续请求快速返回）
var omniInitErr error

// ensureOmniVoice 懒初始化 OmniVoice 引擎：模型目录取自 {LocalDir}/models/OmniVoice
// 模型目录应包含 omnivoice.dll、omnivoice-base-Q8_0.gguf、omnivoice-tokenizer-F32.gguf
// 及其依赖 DLL（vulkan-1/libgomp-1/libwinpthread-1，由 engine/OmniVoiceTTS/build.ps1 就位）
func ensureOmniVoice() error {
	omniInitOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				omniInitErr = fmt.Errorf("OmniVoice 引擎初始化 panic: %v", r)
				LoggerGeneral.Error("CrystalAstral", "%v\n%s", omniInitErr, debug.Stack())
				writeOmniInitError(fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
			}
		}()

		execPath, err := os.Executable()
		if err != nil {
			omniInitErr = err
			return
		}
		execDir := filepath.Dir(execPath)
		localData := filepath.Join(execDir, *GeneralConfig.LocalDir)
		modelDir := filepath.Join(localData, "models", "OmniVoice")

		omniInitErr = OmniVoiceTTS.InitEngine(modelDir)
		if omniInitErr == nil {
			LoggerGeneral.Info("CrystalAstral", "OmniVoice 引擎初始化成功（模型目录: %s）", modelDir)
		} else {
			LoggerGeneral.Error("CrystalAstral", "OmniVoice 引擎初始化失败: %v", omniInitErr)
			writeOmniInitError(omniInitErr.Error())
		}
	})
	return omniInitErr
}

// writeOmniInitError 将引擎初始化错误写入日志文件（琉璃为 windowsgui 无控制台，必须落盘才能排查）
func writeOmniInitError(msg string) {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	logDir := filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "logs")
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "omnivoice_init_error.log")
	_ = os.WriteFile(logPath, []byte(msg+"\n"), 0644)
}

// omniVoiceHandler 包装 OmniVoice 处理器：请求到达时确保引擎已就绪，失败返回 JSON 错误
func omniVoiceHandler(inner http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ensureOmniVoice(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "OmniVoice 引擎初始化失败: " + err.Error(),
			})
			return
		}
		inner(w, r)
	}
}
