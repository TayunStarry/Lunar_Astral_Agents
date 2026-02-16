package utils

import (
	"fmt"  // 格式化输出包，用于字符串格式化
	"net"  // 网络包，用于网络编程
	"time" // 时间包，用于处理时间
)

// CheckPortIsListening 检查指定端口是否被监听
// 参数 port 为要检查的端口号
// 返回值表示端口是否被监听以及检查过程中可能出现的错误
func CheckPortIsListening(port int) (bool, error) {
	// 构造连接地址
	address := fmt.Sprintf("localhost:%d", port)
	// 设置连接超时时间为1秒
	dialer := net.Dialer{
		Timeout: 1 * time.Second,
	}
	// 尝试建立TCP连接
	conn, err := dialer.Dial("tcp", address)
	// 如果连接成功，关闭连接并返回true
	if err == nil {
		conn.Close()
		return true, nil
	}
	// 检查错误类型是否表示端口未被监听
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		// 连接超时，端口未被监听
		return false, nil
	}
	// 返回其他错误
	return false, err
}

// GetFreePort 获取一个空闲端口
// 返回值为空闲端口号以及获取过程中可能出现的错误
func GetFreePort() (int, error) {
	// 监听本地地址的0端口，系统会自动分配一个空闲端口
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	// 监听该端口
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	// 关闭监听
	defer l.Close()
	// 返回分配的端口号
	return l.Addr().(*net.TCPAddr).Port, nil
}

// GetFreePorts 获取多个空闲端口
// 参数 count 为要获取的端口数量
// 返回值为空闲端口号列表以及获取过程中可能出现的错误
func GetFreePorts(count int) ([]int, error) {
	// 用于存储获取到的端口号
	ports := make([]int, 0, count)
	// 循环获取指定数量的端口
	for i := 0; i < count; i++ {
		// 获取一个空闲端口
		port, err := GetFreePort()
		// 如果获取失败，返回错误
		if err != nil {
			return nil, err
		}
		// 将获取到的端口号添加到列表中
		ports = append(ports, port)
	}
	// 返回获取到的端口号列表
	return ports, nil
}
