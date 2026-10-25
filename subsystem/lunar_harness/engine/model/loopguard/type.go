package loopguard

// type.go 思考监管的公共类型定义。

import (
	"context"
	"sync"
	"sync/atomic"
)

// Options 重复检测参数；零值字段回退到默认值
// （1024 字节点 / 每节点 8 个 128 字段 / 平均相似度 0.65）。
type Options struct {
	NodeRunes    int     // 判定节点长度：每输出这么多字检查一次
	SegmentRunes int     // 节点内切分的片段长度
	Similarity   float64 // 相邻片段平均相似度的警戒阈值（0~1）
}

// Guard 流式重复检测守卫：思考与正文独立计数，输出长度每到达一个节点
// （默认 1024 字）就把该节点切成 8 段，比较相邻片段的平均相似度。
// 命中后调用 cancel 中断本次请求，调用方通过 Hit 判断并决定重试或跳过。
type Guard struct {
	mu      sync.Mutex
	texts   map[string][]rune
	checked map[string]int // 每个流已检查到的节点长度
	opts    Options
	cancel  context.CancelFunc
	hitFlag atomic.Bool
}
