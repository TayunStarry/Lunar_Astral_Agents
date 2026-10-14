package portrelease

import (
	"fmt"
	"sync"
)

// logHookFn 日志钩子（TUI 激活时由宿主注入；未注入时回退控制台打印）
var (
	hookMu    sync.Mutex
	logHookFn func(msg string)
)

// SetLogHook 注入日志钩子
func SetLogHook(f func(msg string)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	logHookFn = f
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

// powershellCommands 用于查询指定端口范围 TCP 连接信息的 PowerShell 命令片段
var powershellCommands = []string{
	"$ports = %d..%d",
	"Get-NetTCPConnection -ErrorAction SilentlyContinue | Where-Object { $ports -contains $_.LocalPort } |",
	"Select-Object LocalPort, OwningProcess |",
	"ConvertTo-Csv -NoTypeInformation",
}
