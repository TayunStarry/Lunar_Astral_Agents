package module

import (
	"bytes"
	"fmt"
	"os"
	"strings"
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
		emitEngineLog(w.stream, line)
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
		emitEngineLog(w.stream, rest)
	}
}

// emitEngineLog 输出单行引擎日志：清理控制字符与 ANSI 转义后交钩子；未注入钩子时直写终端
func emitEngineLog(stream, line string) {
	line = cleanEngineLine(line)
	if line == "" {
		return
	}
	if hook := currentEngineLogHook(); hook != nil {
		hook(stream, line)
		return
	}
	fmt.Fprintln(os.Stderr, line)
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
