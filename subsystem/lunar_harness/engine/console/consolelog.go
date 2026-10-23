package console

// ==== 控制台输出的持久化镜像 ====
//
// 为什么需要它：Install 之后 stdout/stderr 全被日志管线接管，所有输出（含 Go 运行时的
// panic 栈）只进 TUI 的内存环形缓冲——进程一退，证据全部消失。真出现「程序自己没了」
// 时，用户与维护者都拿不到任何线索（本项目已经踩过两次）。
//
// 因此把管道扫到的每一行**可选地**落盘：{exeDir}/local_data/logs/console.log，按大小滚动保留
// 最近若干份。日志文件与更新器日志同在 logs 目录，排查时一处即可看全。
//
// 落盘默认关闭：TUI 的「原始日志」视图与「数据导出」已能覆盖排查场景，启动即写文件
// 既多余又容易与导出功能重复。只有 lunar_config.json 的 system.console_log=true 时才开启。

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// consoleLogMaxBytes 单个日志文件上限（超出即滚动）
	consoleLogMaxBytes = 4 << 20
	// consoleLogKeep 滚动保留份数（console.log.1 … .N）
	consoleLogKeep = 3
)

// consoleLogEnabled 控制台日志镜像开关（宿主在 Install 之前从 lunar_config.json 注入；默认关闭）
var consoleLogEnabled bool

// SetConsoleLogEnabled 设置控制台日志镜像开关（须在 Install 之前调用）
func SetConsoleLogEnabled(enabled bool) {
	consoleLogEnabled = enabled
}

// consoleLog 带滚动的控制台日志写入器（并发安全：管道扫描与 panic 恢复都可能写）
type consoleLog struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

// openConsoleLog 打开控制台日志（开关关闭或路径不可用时返回 nil，调用方按无日志处理）
func openConsoleLog() *consoleLog {
	if !consoleLogEnabled {
		return nil
	}
	path := consoleLogPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	size := int64(0)
	if fi, err := f.Stat(); err == nil {
		size = fi.Size()
	}
	return &consoleLog{path: path, f: f, size: size}
}

// consoleLogPath 控制台日志路径（与更新器日志同目录：{程序根}/local_data/logs/）
func consoleLogPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "local_data", "logs", "console.log")
}

// AppendConsoleLog 直接往控制台日志追加一行（带时间戳）。
//
// 与管道镜像的区别：管道只能记录**经由 stdout 打印**的内容，而像「终端字形校准」这类
// 只进 TUI 内存视图的诊断信息必须主动落盘——它是判断「界面为何错位/滚动」的关键证据，
// 用户报障时把它连同日志一起发来即可定位。
func AppendConsoleLog(text string) {
	if !consoleLogEnabled {
		return
	}
	path := consoleLogPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + "  " + text + "\n")
}

// consoleLogWriter 把 logger 的结构化输出直写控制台日志镜像。
//
// TUI 模式下 logger 的打印不进入 stdout 管道：那里是原生库输出、标准库 log 与
// panic 栈的通道，而 logger 自身的日志已经由日志钩子（routeLogRecord）送入
// 「原始日志」视图——再经管道走一遍会让同一行在原始日志里出现两次。
// 落盘职责改由本适配器承担，正文经 scrubLogText 清洗（剥 ANSI、按 \r 分行），
// 级别过滤仍由 logger 的 shouldPrint 把守，console_log 关闭时镜像为 nil、写入静默。
type consoleLogWriter struct {
	cl *consoleLog
}

// Write 实现 io.Writer：log.Logger 每次打印调用一次，正文可能含多行与 \r 进度刷新
func (w consoleLogWriter) Write(p []byte) (int, error) {
	for _, line := range scrubLogText(string(p)) {
		w.cl.WriteLine(line)
	}
	return len(p), nil
}

// WriteLine 追加一行（带时间戳；写失败静默忽略，日志不能影响主流程）
func (c *consoleLog) WriteLine(line string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f == nil {
		return
	}
	stamped := time.Now().Format("2006-01-02 15:04:05") + "  " + line + "\n"
	n, err := c.f.WriteString(stamped)
	if err != nil {
		return
	}
	c.size += int64(n)
	if c.size >= consoleLogMaxBytes {
		c.rotateLocked()
	}
}

// Close 关闭文件句柄
func (c *consoleLog) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f != nil {
		_ = c.f.Close()
		c.f = nil
	}
}

// rotateLocked 滚动：console.log.N-1 → .N，console.log → .1（需持有锁）
func (c *consoleLog) rotateLocked() {
	_ = c.f.Close()
	c.f = nil
	for i := consoleLogKeep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", c.path, i), fmt.Sprintf("%s.%d", c.path, i+1))
	}
	_ = os.Rename(c.path, c.path+".1")

	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	c.f = f
	c.size = 0
}
