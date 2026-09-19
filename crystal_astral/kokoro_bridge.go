package main

import (
	"CrystalAstral/engine/KokoroTTS"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
)

// kokoroInitOnce 引擎懒加载一次性保护（失败后需重启琉璃才能重试）
var kokoroInitOnce sync.Once

// kokoroInitErr 引擎初始化错误（供后续请求快速返回）
var kokoroInitErr error

// ensureKokoro 懒初始化 Kokoro 引擎：模型目录取自 {LocalDir}/models/Kokoro-82M-v1.1-zh
func ensureKokoro() error {
	kokoroInitOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				kokoroInitErr = fmt.Errorf("Kokoro 引擎初始化 panic: %v", r)
				LoggerGeneral.Error("CrystalAstral", "%v\n%s", kokoroInitErr, debug.Stack())
				writeKokoroInitError(fmt.Sprintf("panic: %v\n%s", r, debug.Stack()))
			}
		}()

		execPath, err := os.Executable()
		if err != nil {
			kokoroInitErr = err
			return
		}
		execDir := filepath.Dir(execPath)
		localData := filepath.Join(execDir, *GeneralConfig.LocalDir)
		modelDir := filepath.Join(localData, "models", "Kokoro-82M-v1.1-zh")

		// 指定 onnxruntime.dll（随模型目录放置）
		dllPath := filepath.Join(modelDir, "onnxruntime.dll")
		if _, err := os.Stat(dllPath); err == nil {
			KokoroTTS.SetOnnxLibraryPath(dllPath)
			LoggerGeneral.Info("CrystalAstral", "Kokoro 使用 onnxruntime.dll: %s", dllPath)
		}

		kokoroInitErr = KokoroTTS.InitEngine(
			modelDir,
			filepath.Join(modelDir, "voices"),
			filepath.Join(modelDir, "espeak"),
		)
		if kokoroInitErr == nil {
			engine := KokoroTTS.GetEngine()
			if engine != nil {
				engine.LogAvailable()
			}
		} else {
			LoggerGeneral.Error("CrystalAstral", "Kokoro 引擎初始化失败: %v", kokoroInitErr)
			writeKokoroInitError(kokoroInitErr.Error())
		}
	})
	return kokoroInitErr
}

// writeKokoroInitError 将引擎初始化错误写入日志文件（琉璃为 windowsgui 无控制台，必须落盘才能排查）
func writeKokoroInitError(msg string) {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	logDir := filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "logs")
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "kokoro_init_error.log")
	_ = os.WriteFile(logPath, []byte(msg+"\n"), 0644)
}

// kokoroHandler 包装 Kokoro 处理器：请求到达时确保引擎已就绪，失败返回 JSON 错误
func kokoroHandler(inner http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ensureKokoro(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Kokoro 引擎初始化失败: " + err.Error(),
			})
			return
		}
		inner(w, r)
	}
}
