package module

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	logger "LunarSubsystem/LoggerGeneral"
)

// espeakBinding espeak-ng 可执行文件封装（与参考实现 phonemizer 相同的 CLI 调用方式）
type espeakBinding struct {
	exePath  string
	dataPath string
	mu       sync.Mutex
}

// espeakLib 全局 espeak-ng 绑定实例
var espeakLib *espeakBinding

// initEspeak 初始化 espeak-ng（dir 需包含 espeak-ng.exe 与 espeak-ng-data 目录）
func initEspeak(dir string) bool {
	if dir == "" {
		return false
	}
	exe := filepath.Join(dir, "espeak-ng.exe")
	if _, err := os.Stat(exe); err != nil {
		logger.SubWarn("KOKORO-TTS", "ESPEAK", "espeak-ng.exe 不存在，英文音素化不可用: %s", exe)
		return false
	}
	espeakLib = &espeakBinding{
		exePath:  exe,
		dataPath: filepath.Join(dir, "espeak-ng-data"),
	}
	return true
}

// espeakPhonemize 英文文本 -> IPA 音素（含重音符号）
func espeakPhonemize(text string) string {
	if espeakLib == nil {
		return ""
	}
	espeakLib.mu.Lock()
	defer espeakLib.mu.Unlock()

	cmd := exec.Command(espeakLib.exePath, "-q", "--ipa", "-v", "en-us", text)
	cmd.Env = append([]string{"ESPEAK_DATA_PATH=" + espeakLib.dataPath}, envWithoutEspeakData()...)
	out, err := cmd.Output()
	if err != nil {
		logger.SubWarn("KOKORO-TTS", "ESPEAK", "音素化失败: %v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// envWithoutEspeakData 去除父进程可能设置的 ESPEAK_DATA_PATH，避免覆盖
func envWithoutEspeakData() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ESPEAK_DATA_PATH=") {
			env = append(env, kv)
		}
	}
	return env
}
