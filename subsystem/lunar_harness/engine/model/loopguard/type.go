package loopguard

// type.go 思考监管的公共类型定义。

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options 重复检测参数；零值字段回退到默认值（64 字窗口 / 3 窗口 / 0.5 / 1 秒）。
type Options struct {
	WindowRunes int           // 单个窗口的字符数
	WindowCount int           // 判定重复所需的连续窗口数
	Similarity  float64       // 相似度阈值（0~1）
	Interval    time.Duration // 后台检查间隔
}

// Guard 流式重复检测守卫：把思考与正文增量喂进来，后台定期检查是否卡在循环。
// 命中后调用 cancel 中断本次请求，调用方通过 Hit 判断并决定重试或跳过。
type Guard struct {
	mu      sync.Mutex
	texts   map[string]*strings.Builder
	opts    Options
	cancel  context.CancelFunc
	stopCh  chan struct{}
	doneCh  chan struct{}
	hitFlag atomic.Bool
}
