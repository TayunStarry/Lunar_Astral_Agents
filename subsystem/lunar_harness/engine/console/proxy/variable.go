package proxy

import (
	"fmt"
	"sync"
)

// 证书文件路径（宿主可通过 SetCertPaths 覆盖；默认落在工作目录 local_data/cert/ 下）
var (
	certMu    sync.RWMutex
	certFile  = "local_data/cert/proxy_cert.pem"
	keyFile   = "local_data/cert/proxy_key.pem"
	logHookFn func(msg string)
	hookMu    sync.Mutex
)

// SetCertPaths 配置 TLS 证书与私钥路径
func SetCertPaths(cert, key string) {
	certMu.Lock()
	defer certMu.Unlock()
	certFile, keyFile = cert, key
}

func getCertPaths() (string, string) {
	certMu.RLock()
	defer certMu.RUnlock()
	return certFile, keyFile
}

// SetLogHook 注入日志钩子（TUI 激活时由宿主注入；未注入时回退控制台打印）
func SetLogHook(f func(msg string)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	logHookFn = f
}

// logf 统一日志出口：钩子优先，回退控制台
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
