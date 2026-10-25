package sound

// sound.go 共享提示音：警告/错误播放失败音效，资源产出完成播放完成音效。
//
// 播放实现按平台拆分（Windows 走 winmm MCI，其他平台静默跳过）。
// 失败音效按 10 秒、完成音效按 3 秒节流，避免告警风暴或短时间连续产出时反复响。

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// Failure 异步播放失败提示音（警告及以上、服务启动失败、端口被占用）
func Failure() {
	trigger(KindFailure)
}

// FailureBlocking 同步播放失败提示音：用于即将退出进程的致命错误，保证声音播完再退出
func FailureBlocking() {
	playFile(fileNameOf(KindFailure), true)
}

// Notify 异步播放资源产出完成提示音（图像生成、上传、打包归档）
func Notify() {
	trigger(KindNotify)
}

// trigger 按冷却与播放状态决定是否异步播放
func trigger(kind Kind) {
	now := time.Now()
	stateMu.Lock()
	if playing[kind] || now.Sub(lastPlayed[kind]) < cooldownOf(kind) {
		stateMu.Unlock()
		return
	}
	playing[kind] = true
	lastPlayed[kind] = now
	stateMu.Unlock()

	file := fileNameOf(kind)
	go func() {
		defer func() {
			stateMu.Lock()
			playing[kind] = false
			stateMu.Unlock()
		}()
		playFile(file, true)
	}()
}

// cooldownOf 返回该提示音的冷却时间
func cooldownOf(kind Kind) time.Duration {
	if kind == KindFailure {
		return failureCooldown
	}
	return notifyCooldown
}

// fileNameOf 返回该提示音的文件名
func fileNameOf(kind Kind) string {
	if kind == KindFailure {
		return failureFile
	}
	return notifyFile
}

// audioPath 解析 local_data/audios/<name> 的绝对路径。
//
// LocalDir 为绝对路径时直接使用；否则以可执行文件目录为基准；
// 开发态（exe 在临时构建目录）再回退当前工作目录。找不到文件返回 false。
func audioPath(name string) (string, bool) {
	dir := strings.TrimSpace(config.LocalDir)
	if dir == "" {
		dir = "local_data"
	}
	if !filepath.IsAbs(dir) {
		if exe, err := os.Executable(); err == nil {
			dir = filepath.Join(filepath.Dir(exe), dir)
		}
	}
	if path := filepath.Join(dir, "audios", name); fileExists(path) {
		return path, true
	}
	if path := filepath.Join("local_data", "audios", name); fileExists(path) {
		return path, true
	}
	return "", false
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
