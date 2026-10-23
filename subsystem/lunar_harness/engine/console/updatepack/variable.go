package updatepack

import (
	"fmt"
	"io"
	"sync"
)

// giteeAPIBase Gitee OpenAPI v5 基地址。
// 变量而非常量：单测把它指向本地 httptest 服务，从而验证「取最新」的排序与筛选逻辑
// （不依赖真实网络，也不需要令牌）。
var giteeAPIBase = "https://gitee.com/api/v5"

// logHookFn 日志钩子（TUI 激活时由宿主注入；未注入时回退控制台打印）
var (
	hookMu       sync.Mutex
	logHookFn    func(msg string)
	progressHook func(percent int)
)

// SetLogHook 注入日志钩子
func SetLogHook(f func(msg string)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	logHookFn = f
}

// SetProgressHook 注入进度回调钩子（percent: 0-100；-1 表示未知进度）
func SetProgressHook(f func(percent int)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	progressHook = f
}

// logf 统一日志出口
func logf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	hookMu.Lock()
	hook := logHookFn
	hookMu.Unlock()
	if hook != nil {
		hook(msg)
		return
	}
	fmt.Println(msg)
}

// reportProgress 上报进度（未注入钩子时静默跳过）
func reportProgress(percent int) {
	hookMu.Lock()
	hook := progressHook
	hookMu.Unlock()
	if hook != nil {
		hook(percent)
	}
}

// consoleWriterFn 真实终端输出提供者（宿主注入；未注入时返回 nil）。
// 用途：更新器子进程的进度接回用户看得见的终端——主程序接管 stdout 后，
// 子进程若继承 NUL 设备，用户就完全看不到更新在发生。
var consoleWriterFn func() io.Writer

// SetConsoleWriter 注入真实终端输出（控制台启动时调用）
func SetConsoleWriter(f func() io.Writer) {
	hookMu.Lock()
	defer hookMu.Unlock()
	consoleWriterFn = f
}

// consoleWriter 取真实终端输出（未注入时返回 nil，由调用方回退默认行为）
func consoleWriter() io.Writer {
	hookMu.Lock()
	defer hookMu.Unlock()
	if consoleWriterFn == nil {
		return nil
	}
	return consoleWriterFn()
}
