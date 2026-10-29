package archive

import "fmt"

// logHookFn 日志输出钩子（TUI 激活时由宿主注入，未注入时回退控制台打印）
var logHookFn func(msg string)

// progressHookFn 进度回调钩子（percent: 0-100；-1 表示准备阶段未知进度）
var progressHookFn func(percent int)

// maintenanceHook 维护包装钩子（宿主注入：打包期间暂停思维链并卸载模型）
var maintenanceHook func(fn func() error) error

// SetLogHook 注入日志输出钩子
func SetLogHook(f func(msg string)) { logHookFn = f }

// SetProgressHook 注入进度回调钩子
func SetProgressHook(f func(percent int)) { progressHookFn = f }

// SetMaintenanceHook 注入维护包装钩子（未注入时直接执行原任务）
func SetMaintenanceHook(f func(fn func() error) error) { maintenanceHook = f }

// runMaintenance 在维护钩子内执行 fn；未注入钩子时直接执行。
func runMaintenance(fn func() error) error {
	if maintenanceHook == nil {
		return fn()
	}
	return maintenanceHook(fn)
}

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
