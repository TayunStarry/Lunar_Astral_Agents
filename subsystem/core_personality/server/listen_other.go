//go:build !windows

package server

import "syscall"

// exclusiveAddrControl 非 Windows 平台的空实现。
// Unix 的 bind 本身即独占（同一地址:端口重复绑定会失败），无需额外套接字选项。
func exclusiveAddrControl(network, address string, c syscall.RawConn) error {
	return nil
}
