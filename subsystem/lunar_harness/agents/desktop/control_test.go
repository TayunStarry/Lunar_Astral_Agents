//go:build windows

package desktop

// control_test.go 钉住桌面调用超时的行为：超时返回普通错误，取消返回 ErrCanceled。

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallDesktopHonorsCancelAndTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := callDesktop(ctx, time.Second, "测试", func() (int, error) { return 1, nil }); !errors.Is(err, ErrCanceled) {
		t.Fatalf("已取消的上下文应返回 ErrCanceled，实际 %v", err)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	started := time.Now()
	_, err := callDesktop(ctx2, 30*time.Millisecond, "阻塞测试", func() (int, error) {
		time.Sleep(200 * time.Millisecond)
		return 1, nil
	})
	if err == nil || errors.Is(err, ErrCanceled) {
		t.Fatalf("超时应返回普通错误而不是 ErrCanceled，实际 %v", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("超时保护没有及时返回，耗时 %s", elapsed)
	}
}
