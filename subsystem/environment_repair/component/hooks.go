package component

import "fmt"

// uiLogHook 日志输出钩子（TUI 激活时由宿主注入，未注入时回退控制台打印）
var uiLogHook func(msg string)

// progressHook 进度回调钩子（percent: 0-100；-1 表示准备阶段未知进度）
var progressHook func(percent int)

// SetUiLogHook 注入日志输出钩子
func SetUiLogHook(f func(msg string)) { uiLogHook = f }

// SetProgressHook 注入进度回调钩子
func SetProgressHook(f func(percent int)) { progressHook = f }

// uiLogf 组件内部统一日志出口：钩子优先，回退控制台
func uiLogf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	if uiLogHook != nil {
		uiLogHook(msg)
		return
	}
	fmt.Println(msg)
}

// reportProgress 上报进度（未注入钩子时静默跳过）
func reportProgress(percent int) {
	if progressHook != nil {
		progressHook(percent)
	}
}
