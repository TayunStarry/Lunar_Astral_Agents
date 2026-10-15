//go:build windows

package server

import (
	"syscall"
)

// soExclusiveAddrUse Winsock 的 SO_EXCLUSIVEADDRUSE。
//
// Winsock2.h 将其定义为 ~SO_REUSEADDR（SO_REUSEADDR = 4），即 -5 / 0xFFFFFFFB；
// Go 的 syscall 与 x/sys/windows 都未导出该常量，故按定义直书。
const soExclusiveAddrUse = -5

// exclusiveAddrControl 为监听套接字设置 SO_EXCLUSIVEADDRUSE。
//
// Windows 的默认语义允许同一地址:端口被多个套接字绑定（后者接管新连接），
// 且 Go 不再替我们设置该选项——必须显式设置，否则「重复启动」不会被拦下，
// 两个实例会同时对外服务并互抢模型端口。
func exclusiveAddrControl(network, address string, c syscall.RawConn) error {
	var sockErr error
	if err := c.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); err != nil {
		return err
	}
	return sockErr
}
