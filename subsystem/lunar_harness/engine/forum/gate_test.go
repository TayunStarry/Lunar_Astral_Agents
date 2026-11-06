package forum

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireWriteSerializesWriters(t *testing.T) {
	const workers = 8
	var active, peak int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := AcquireWrite(context.Background())
			if err != nil {
				t.Errorf("获取写锁失败: %v", err)
				return
			}
			defer release()

			current := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&peak)
				if current <= old || atomic.CompareAndSwapInt32(&peak, old, current) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got != 1 {
		t.Fatalf("写锁未串行化：最大并发 %d", got)
	}
}

func TestAcquireWriteHonorsContext(t *testing.T) {
	release, err := AcquireWrite(context.Background())
	if err != nil {
		t.Fatalf("获取写锁失败: %v", err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if second, acquireErr := AcquireWrite(ctx); acquireErr == nil {
		second()
		t.Fatal("写锁已被占用，不应获取成功")
	} else if !errors.Is(acquireErr, context.DeadlineExceeded) {
		t.Fatalf("期望等待超时，实际 %v", acquireErr)
	}

	release()
	released = true

	again, err := AcquireWrite(context.Background())
	if err != nil {
		t.Fatalf("释放后应能再次获取: %v", err)
	}
	again()
}
