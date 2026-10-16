//go:build windows

package proxy

import "syscall"

// soExclusiveAddrUse Winsock 的 SO_EXCLUSIVEADDRUSE（= ~SO_REUSEADDR = -5）。
// Go 的 syscall 与 x/sys/windows 都未导出该常量，按 Winsock 定义直书。
const soExclusiveAddrUse = -5

// exclusiveAddrControl 为监听套接字设置 SO_EXCLUSIVEADDRUSE，
// 使「端口已被占用」必然表现为绑定失败，而不是被静默接管。
func exclusiveAddrControl(network, address string, c syscall.RawConn) error {
	var sockErr error
	if err := c.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); err != nil {
		return err
	}
	return sockErr
}
