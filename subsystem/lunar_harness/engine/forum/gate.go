package forum

// gate.go 论坛写路径的进程内唯一互斥点。
//
// 归属：engine/forum 是 agents 与 chain 都能依赖的最低共享层，engine 不反向依赖
// agents/chain；因此把锁放在这里，两条路径拿到的必然是同一把锁。
//
// 覆盖范围只包括会改动论坛或本地账号状态的写路径：申领、轮次领取、发帖、回复、
// 评论、点赞、投票、改写、删除、改名、事件游标推进。健康探测、列表、详情、检索
// 等只读请求不经过这里，因而不会被写任务长时间阻塞。
//
// 使用带缓冲通道而非 sync.Mutex：等待方可以随 ctx 取消退出，避免一条长任务把
// 另一条路径无限期挂住。持锁范围应只覆盖「读取账号状态 → 请求服务端 → 落盘」，
// 生成回复等长耗时工作必须在锁外完成。

import "context"

const writeGateCapacity = 1

// writeGate 就是唯一的论坛写锁：通道里有一个令牌表示空闲。
var writeGate = make(chan struct{}, writeGateCapacity)

func init() {
	writeGate <- struct{}{}
}

// AcquireWrite 获取论坛写锁；ctx 取消时返回 ctx.Err()。
// 返回的 release 必须且只能调用一次。
func AcquireWrite(ctx context.Context) (release func(), err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-writeGate:
		return func() { writeGate <- struct{}{} }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WithWrite 在论坛写锁内执行 fn，并在结束时释放。
func WithWrite[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	release, err := AcquireWrite(ctx)
	if err != nil {
		return zero, err
	}
	defer release()
	return fn()
}

// WithWriteErr 是 WithWrite 的 error-only 形式。
func WithWriteErr(ctx context.Context, fn func() error) error {
	_, err := WithWrite(ctx, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
