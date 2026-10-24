package loopguard

// loopguard.go 共享思考监管：模型卡在循环重复输出时中断本次调用。
//
// 机制：流式输出（思考与正文分开累积）每秒检查一次最近 3 个窗口；相邻窗口的
// 字符二元组重合度（Dice）连续两次超过阈值即判定模型在原地重复，立即调用
// cancel 中断请求。命中后由各智能体按自己的任务编排决定重试、降级或跳过。
//
// 窗口按不重叠的整块切分：若用逐字滚动窗口做比较，相邻窗口天然共享绝大部分
// 字符，任何正常文本的相似度都会接近 1，必然误判。

import (
	"context"
	"strings"
	"time"
)

// DefaultOptions 返回默认检测参数
func DefaultOptions() Options {
	return Options{
		WindowRunes: DefaultWindowRunes,
		WindowCount: DefaultWindowCount,
		Similarity:  DefaultSimilarity,
		Interval:    DefaultInterval,
	}
}

// normalize 用默认值补齐零值字段
func (opts Options) normalize() Options {
	if opts.WindowRunes <= 0 {
		opts.WindowRunes = DefaultWindowRunes
	}
	if opts.WindowCount < 2 {
		opts.WindowCount = DefaultWindowCount
	}
	if opts.Similarity <= 0 || opts.Similarity >= 1 {
		opts.Similarity = DefaultSimilarity
	}
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	return opts
}

// New 创建守卫；cancel 用于命中重复时中断本次请求
func New(cancel context.CancelFunc, opts Options) *Guard {
	return &Guard{
		texts: map[string]*strings.Builder{
			KindThink:   {},
			KindContent: {},
		},
		opts:   opts.normalize(),
		cancel: cancel,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Feed 追加一段流式文本
func (guard *Guard) Feed(kind, text string) {
	if guard == nil || text == "" {
		return
	}
	guard.mu.Lock()
	defer guard.mu.Unlock()
	buffer, ok := guard.texts[kind]
	if !ok {
		buffer = &strings.Builder{}
		guard.texts[kind] = buffer
	}
	buffer.WriteString(text)
}

// Start 启动定期检查
func (guard *Guard) Start() {
	if guard == nil {
		return
	}
	go func() {
		defer close(guard.doneCh)
		ticker := time.NewTicker(guard.opts.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-guard.stopCh:
				return
			case <-ticker.C:
				if guard.detect() {
					guard.hitFlag.Store(true)
					guard.cancel()
					return
				}
			}
		}
	}()
}

// Stop 停止检查并等待后台退出，最后补做一次检查（避免末段重复漏判）
func (guard *Guard) Stop() {
	if guard == nil {
		return
	}
	close(guard.stopCh)
	<-guard.doneCh
	if guard.detect() {
		guard.hitFlag.Store(true)
		guard.cancel()
	}
}

// Hit 本次输出是否因重复被中断
func (guard *Guard) Hit() bool {
	return guard != nil && guard.hitFlag.Load()
}

// detect 检查全部流是否都已卡在循环里
func (guard *Guard) detect() bool {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	for _, buffer := range guard.texts {
		if Detected(buffer.String(), guard.opts) {
			return true
		}
	}
	return false
}

// Detected 判断文本尾部是否连续多个窗口高度相似（非流式响应的静态检查也用它）
func Detected(text string, opts Options) bool {
	opts = opts.normalize()
	runes := []rune(text)
	size := opts.WindowRunes * opts.WindowCount
	if len(runes) < size {
		return false
	}
	tail := runes[len(runes)-size:]
	previous := string(tail[:opts.WindowRunes])
	for index := 1; index < opts.WindowCount; index++ {
		current := string(tail[index*opts.WindowRunes : (index+1)*opts.WindowRunes])
		if Dice(previous, current) <= opts.Similarity {
			return false
		}
		previous = current
	}
	return true
}

// Dice 计算两段文本的字符二元组重合度（0~1）
//
// 用二元组而不是逐位比对：循环重复不一定与窗口对齐（句子长度很少整除窗口），
// 逐位比对会漏判；二元组重合度对错位重复依然敏感。
func Dice(left, right string) float64 {
	runesLeft, runesRight := []rune(left), []rune(right)
	if len(runesLeft) < 2 || len(runesRight) < 2 {
		return 0
	}
	setLeft := bigramSet(runesLeft)
	setRight := bigramSet(runesRight)
	shared := 0
	for gram := range setLeft {
		if _, ok := setRight[gram]; ok {
			shared++
		}
	}
	return float64(2*shared) / float64(len(setLeft)+len(setRight))
}

// bigramSet 提取字符二元组集合
func bigramSet(runes []rune) map[string]struct{} {
	set := make(map[string]struct{}, len(runes)-1)
	for i := 1; i < len(runes); i++ {
		set[string(runes[i-1:i+1])] = struct{}{}
	}
	return set
}
