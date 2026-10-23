package archive

import "fmt"

// logHookFn 日志输出钩子（TUI 激活时由宿主注入，未注入时回退控制台打印）
var logHookFn func(msg string)

// progressHookFn 进度回调钩子（percent: 0-100；-1 表示准备阶段未知进度）
var progressHookFn func(percent int)

// SetLogHook 注入日志输出钩子
func SetLogHook(f func(msg string)) { logHookFn = f }

// SetProgressHook 注入进度回调钩子
func SetProgressHook(f func(percent int)) { progressHookFn = f }

// logf 组件内部统一日志出口：钩子优先，回退控制台
func logf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	if logHookFn != nil {
		logHookFn(msg)
		return
	}
	fmt.Println(msg)
}

// reportProgress 上报进度（未注入钩子时静默跳过）
func reportProgress(percent int) {
	if progressHookFn != nil {
		progressHookFn(percent)
	}
}
