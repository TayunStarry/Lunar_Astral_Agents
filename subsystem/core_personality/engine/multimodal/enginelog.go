package multimodal

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"LunarSubsystem/LoggerGeneral"
)

// SetEngineLogHook 注入引擎日志钩子（TUI 构建调用；传 nil 注销）。
// line 为已切分并清理控制字符的单行文本，stream 为 stdout/stderr。
func SetEngineLogHook(f func(stream, line string)) {
	engineLogMu.Lock()
	engineLogHook = f
	engineLogMu.Unlock()
}

// currentEngineLogHook 读取当前钩子
func currentEngineLogHook() func(stream, line string) {
	engineLogMu.RLock()
	defer engineLogMu.RUnlock()
	return engineLogHook
}

// newEngineLogWriter 创建引擎输出流的分行写入器
func newEngineLogWriter(stream string, fallback *os.File) *engineLogWriter {
	return &engineLogWriter{stream: stream, fallback: fallback}
}

// Write 写入引擎输出：无钩子时直写原始终端流（保持进度条原地刷新）；
// 有钩子时按行切分（\n 与 \r 均作换行）后逐行回调，供宿主统一排版。
func (w *engineLogWriter) Write(p []byte) (int, error) {
	if currentEngineLogHook() == nil {
		return w.fallback.Write(p)
	}

	w.mu.Lock()
	w.buf = append(w.buf, p...)
	var lines []string
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		lines = append(lines, string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	w.mu.Unlock()

	for _, line := range lines {
		w.emit(line)
	}
	return len(p), nil
}

// Close 冲刷残余未成行的内容（引擎退出后调用）
func (w *engineLogWriter) Close() {
	w.mu.Lock()
	rest := string(w.buf)
	w.buf = nil
	w.mu.Unlock()
	if rest != "" {
		w.emit(rest)
	}
}

// vaeTilingFallbackMark sd.cpp 在 VAE 解码显存不足时自行改用空间分片解码的日志标记
// （backend_fit.cpp:506）。这条降级**不受调用方控制**：没传 --vae-tiling 也会发生。
const vaeTilingFallbackMark = "retrying with spatial tiling"

// emit 输出单行引擎日志：清理控制字符与 ANSI 转义后交钩子；未注入钩子时直写终端。
//
// 顺带盯一件事：VAE 解码被引擎自行降级为空间分片时，wan_vae 的分片接缝会在画面上
// 留下硬边纯色方块（实测 17×16 px 的白块，边界正落在 512px = 32 latent 的分片线上）。
// 用户只看到白块、看不到原因，所以这里把它翻成一条显式警告（每次生成只报一次）。
func (w *engineLogWriter) emit(line string) {
	line = cleanEngineLine(line)
	if line == "" {
		return
	}
	if strings.Contains(line, vaeTilingFallbackMark) {
		w.warnVAETilingFallback()
	}
	if hook := currentEngineLogHook(); hook != nil {
		hook(w.stream, line)
		return
	}
	fmt.Fprintln(os.Stderr, line)
}

// warnVAETilingFallback 报告「VAE 解码被引擎自行降级为分片」（同一引擎进程只报一次）
func (w *engineLogWriter) warnVAETilingFallback() {
	if w.stream != "stdout" {
		return // 该标记只出现在引擎 stdout
	}
	w.mu.Lock()
	already := w.vaeFallbackWarned
	w.vaeFallbackWarned = true
	w.mu.Unlock()
	if already {
		return
	}
	LoggerGeneral.Warn("MultimodalAnalysis",
		"VAE 解码显存不足，引擎已自行改用空间分片：画面接缝处可能出现硬边纯色方块。"+
			"建议把 sd_vae_backend 设为 cpu（整段 CPU 解码）或降低出图分辨率")
}

// cleanEngineLine 清理单行文本：去除 ANSI 转义序列与其余控制字符，
// 压缩连续空白（引擎输出常带大量用于对齐的空格），去首尾空白。
func cleanEngineLine(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		// ANSI CSI：ESC [ ... 终止字母
		if r == 0x1b {
			j := i + 1
			if j < len(rs) && rs[j] == '[' {
				j++
				for j < len(rs) && !((rs[j] >= 'a' && rs[j] <= 'z') || (rs[j] >= 'A' && rs[j] <= 'Z')) {
					j++
				}
			}
			i = j
			continue
		}
		// 其余控制字符（不含制表符）直接丢弃
		if r < 0x20 && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	// 压缩连续空白，便于终端排版
	return strings.Join(strings.Fields(b.String()), " ")
}
