package viewer

// watchlist_budget.go 待看列表补齐管线的时间预算。
//
// 预算分三层：整体上限、采集阶段份额（总时长的 2/3）与单候选子预算。
// 到点后已入队/已下载的成果保留，未开始的候选不产生任何写入，避免与对话回合长时间争抢资源。

import (
	"context"
	"time"
)

// newWatchlistBudget 归一化预算并把起点设为当前时刻；非正数取机制默认值。
func newWatchlistBudget(total, perItem time.Duration, now func() time.Time) watchlistBudget {
	if now == nil {
		now = time.Now
	}
	if total <= 0 {
		total = watchlistBudgetDefault
	}
	if perItem <= 0 {
		perItem = watchlistCandidateBudgetDefault
	}
	if perItem > total {
		perItem = total
	}
	return watchlistBudget{total: total, perItem: perItem, now: now, start: now()}
}

// collectExpired 采集阶段是否已用完自己的份额（留出余量给审核与下载）。
func (b watchlistBudget) collectExpired() bool {
	return b.limitExpired(b.total * watchlistCollectShareNum / watchlistCollectShareDen)
}

// expired 整体预算是否已耗尽。
func (b watchlistBudget) expired() bool {
	return b.limitExpired(b.total)
}

// limitExpired 指定上限是否已耗尽。
func (b watchlistBudget) limitExpired(limit time.Duration) bool {
	return limit > 0 && b.now().Sub(b.start) >= limit
}

// itemContext 为单个候选派生带子预算的上下文；子预算同时受整体剩余时长约束，
// 整体已到点（边界竞态）时至少给 1 秒收尾，避免把单条工作直接判定为半途失败。
func (b watchlistBudget) itemContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := b.perItem
	if left := b.total - b.now().Sub(b.start); left < timeout {
		timeout = left
	}
	if timeout < time.Second {
		timeout = time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
