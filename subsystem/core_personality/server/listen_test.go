package server

import (
	"net"
	"runtime"
	"testing"
)

// TestListenInstanceExclusive 单实例闸门的核心契约：
// 同一端口第二次独占绑定必须失败。
//
// 为什么必须在测试里钉住：Windows 默认允许同一地址:端口被多个套接字绑定（后绑定者接管
// 新连接），而 Go 自 1.19 起不再设置 SO_EXCLUSIVEADDRUSE。缺了这道闸门，重复启动的两个
// 实例会同时"监听成功"、各自拉起一个模型服务并互抢端口，用户看到的是
// 「前端 404 → 前后端一起崩溃退出」。
func TestListenInstanceExclusive(t *testing.T) {
	first, err := ListenInstance(0, false)
	if err != nil {
		t.Fatalf("首次独占绑定应成功: %v", err)
	}
	defer first.Close()

	port := first.Addr().(*net.TCPAddr).Port
	if port == 0 {
		t.Fatalf("未能取得监听端口")
	}

	second, err := ListenInstance(port, false)
	if err == nil {
		_ = second.Close()
		if runtime.GOOS == "windows" {
			t.Fatalf("端口 %d 已被独占绑定，第二次绑定必须失败（否则重复启动会同时对外服务）", port)
		}
		t.Skipf("非 Windows 平台语义不同：第二次绑定被系统接受")
	}
	if second != nil {
		_ = second.Close()
		t.Fatalf("失败时不应返回监听器")
	}
}

// TestPreboundListenerConsumedOnce 预绑定监听器只能被取用一次
// （否则第二次 Start 会复用一个已关闭的监听器）
func TestPreboundListenerConsumedOnce(t *testing.T) {
	ln, err := ListenInstance(0, false)
	if err != nil {
		t.Fatalf("绑定失败: %v", err)
	}
	defer ln.Close()

	SetPreboundListener(ln)
	if got := takePreboundListener(); got != ln {
		t.Fatalf("首次取用应返回注入的监听器")
	}
	if got := takePreboundListener(); got != nil {
		t.Fatalf("再次取用应为空，实际 %v", got)
	}
}
