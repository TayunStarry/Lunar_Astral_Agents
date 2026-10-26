//go:build windows

package desktop

// timeout.go 给可能长时间阻塞的桌面原语加一层上下文与超时保护。
//
// Windows UIA / 键鼠 / 截图调用没有原生的 ctx 取消接口；一旦驱动层卡住，
// 外层只能靠超时把控制权还给编排循环。超时后的底层调用可能仍会在后台结束，
// 但结果通过带缓冲的 channel 丢弃，不会阻塞 Run，也不会污染新一轮任务。

import (
	"context"
	"fmt"
	"time"
)

type desktopCallResult[T any] struct {
	value T
	err   error
}

// callDesktop 在 ctx 取消或超时后立即返回；fn 在独立 goroutine 中执行。
func callDesktop[T any](ctx context.Context, timeout time.Duration, label string, fn func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, ErrCanceled
	}
	done := make(chan desktopCallResult[T], 1)
	go func() {
		value, err := fn()
		done <- desktopCallResult[T]{value: value, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-done:
		return result.value, result.err
	case <-ctx.Done():
		return zero, ErrCanceled
	case <-timer.C:
		return zero, fmt.Errorf("%s超时（%s）", label, timeout)
	}
}
