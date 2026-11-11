package MutualForum

// gate_test.go 验证论坛唯一写锁：写路径串行、读路径不被阻塞、自动回应长任务不饿死其它路径。

import (
	"context"
	"errors"
	"testing"
	"time"
)

func prepareGateTest(t *testing.T) *fakeForum {
	t.Helper()
	forum := newFakeForum(t)
	installConfig(t, testConfig(t, forum.URL()))
	if _, err := Identity(context.Background()); err != nil {
		t.Fatalf("准备身份失败: %v", err)
	}
	return forum
}

func requestCount(forum *fakeForum) int {
	forum.mu.Lock()
	defer forum.mu.Unlock()
	return forum.requestCount
}

func TestWriteOperationWaitsForSharedGate(t *testing.T) {
	forum := prepareGateTest(t)
	before := requestCount(forum)

	release, err := acquireWrite(context.Background())
	if err != nil {
		t.Fatalf("获取写锁失败: %v", err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			release()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if _, err := CreateTopic(ctx, "并发写测试", "持锁期间不应发出写请求。", ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("持锁期间写操作应等待并超时，实际 %v", err)
	}
	if after := requestCount(forum); after != before {
		t.Fatalf("持锁期间不应发出请求：before=%d after=%d", before, after)
	}

	release()
	released = true
	if _, err := CreateTopic(context.Background(), "并发写测试", "释放写锁后应成功。", ""); err != nil {
		t.Fatalf("释放写锁后写操作应成功: %v", err)
	}
}

func TestReadOperationNotBlockedBySharedGate(t *testing.T) {
	_ = prepareGateTest(t)

	release, err := acquireWrite(context.Background())
	if err != nil {
		t.Fatalf("获取写锁失败: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if _, err := ListTopics(ctx, "open", 0, 10); err != nil {
		t.Fatalf("读操作不应被写锁阻塞: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("读操作等待过久: %v", elapsed)
	}
}

func TestAutoReplySendUsesSharedGate(t *testing.T) {
	_ = prepareGateTest(t)

	release, err := acquireWrite(context.Background())
	if err != nil {
		t.Fatalf("获取写锁失败: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	item := unreadReply{EventSeq: 1, Type: "comment_created", TopicID: "t_0123abcd"}
	if err := sendAutoReply(ctx, item, "回复内容"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("自动回应提交应进入同一写锁并等待超时，实际 %v", err)
	}
}

func TestAutoRespondGenerationDoesNotHoldSharedGate(t *testing.T) {
	item := unreadReply{EventSeq: 1, Type: "comment_created", TopicID: "t_0123abcd"}
	fake := newFakeRespondDeps(item)
	deps := fake.deps(3)

	started := make(chan struct{})
	unblock := make(chan struct{})
	deps.generate = func(context.Context, replyContext) (string, error) {
		close(started)
		<-unblock
		return "回复内容", nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = autoRespond(context.Background(), deps)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("自动回应未进入生成阶段")
	}

	acquired := make(chan struct{})
	go func() {
		gateRelease, err := acquireWrite(context.Background())
		if err != nil {
			return
		}
		close(acquired)
		gateRelease()
	}()
	select {
	case <-acquired:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("自动回应生成期间不应持有论坛写锁")
	}

	close(unblock)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("自动回应未结束")
	}
}
