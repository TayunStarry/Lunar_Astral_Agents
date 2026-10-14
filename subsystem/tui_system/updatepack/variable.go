package updatepack

import (
	"fmt"
	"sync"
)

// giteeAPIBase Gitee OpenAPI v5 基地址
const giteeAPIBase = "https://gitee.com/api/v5"

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
