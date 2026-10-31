package loopguard

// loopguard.go 共享思考监管：模型卡在循环重复输出时中断本次调用。
//
// 判定方式（按实际转写样本标定）：
//   - 思考与正文独立计数；
//   - 每个流每输出 DefaultNodeRunes（1024）字检查一次，不做每秒轮询；
//   - 把刚完成的节点切成 8 个 128 字片段，算相邻片段的平均 Dice 相似度；
//   - 平均相似度超过阈值（默认 0.65）即判定为持续复读，立即 cancel。
//
// 为什么要「节点 + 平均」：短时间的模板化枚举（几帧 Same as Frame8）只影响
// 一两个片段，平均会被正常内容稀释；而真正卡在复读时，整个 1024 字段都高度
// 相似。这样既能放过静态画面的正常逐帧描述，又能抓住连续几百行的复读。
//
// 阈值可用 lunar_config.json 的 system.loop_* 覆盖（见 engine/config）。

import (
	"context"
	"unicode"

	"LunarSubsystem/LunarHarness/engine/config"
)

// DefaultOptions 返回默认检测参数（system.loop_* 有配置时优先使用配置值）
func DefaultOptions() Options {
	opts := Options{
		NodeRunes:    DefaultNodeRunes,
		SegmentRunes: DefaultSegmentRunes,
		Similarity:   DefaultSimilarity,
	}
	if config.LoopNodeRunes > 0 {
		opts.NodeRunes = config.LoopNodeRunes
	}
	if config.LoopSegmentRunes > 0 {
		opts.SegmentRunes = config.LoopSegmentRunes
	}
	if config.LoopSimilarity > 0 && config.LoopSimilarity < 1 {
		opts.Similarity = config.LoopSimilarity
	}
	return opts
}

// normalize 用默认值补齐零值字段
func (opts Options) normalize() Options {
	if opts.NodeRunes <= 0 {
		opts.NodeRunes = DefaultNodeRunes
	}
	if opts.SegmentRunes <= 0 {
		opts.SegmentRunes = DefaultSegmentRunes
	}
	if opts.SegmentRunes*2 > opts.NodeRunes {
		opts.SegmentRunes = opts.NodeRunes / 2
	}
	if opts.Similarity <= 0 || opts.Similarity >= 1 {
		opts.Similarity = DefaultSimilarity
	}
	return opts
}

// New 创建守卫；cancel 用于命中重复时中断本次请求
func New(cancel context.CancelFunc, opts Options) *Guard {
	return &Guard{
		texts: map[string][]rune{
			KindThink:   {},
			KindContent: {},
		},
		checked: map[string]int{
			KindThink:   0,
			KindContent: 0,
		},
		opts:   opts.normalize(),
		cancel: cancel,
	}
}

// Feed 追加一段流式文本；跨过节点边界时立即检查刚完成的那个节点。
func (g *Guard) Feed(kind, text string) {
	if g == nil || text == "" || g.hitFlag.Load() {
		return
	}
	g.mu.Lock()
	g.texts[kind] = append(g.texts[kind], []rune(text)...)
	length := len(g.texts[kind])
	next := length / g.opts.NodeRunes * g.opts.NodeRunes
	var windows [][]rune
	for checked := g.checked[kind]; checked < next; checked += g.opts.NodeRunes {
		start := checked
		window := append([]rune(nil), g.texts[kind][start:start+g.opts.NodeRunes]...)
		windows = append(windows, window)
	}
	if next > g.checked[kind] {
		g.checked[kind] = next
	}
	g.mu.Unlock()

	for _, window := range windows {
		if blockRepeats(window, g.opts) {
			g.hitFlag.Store(true)
			g.cancel()
			return
		}
	}
}

// Stop 流结束时的兜底检查：模型可能在节点中间卡住并以空内容收尾，
// 这里对最后一个完整节点长度（不足则跳过）再判一次，避免末段漏判。
func (g *Guard) Stop() {
	if g == nil || g.hitFlag.Load() {
		return
	}
	g.mu.Lock()
	windows := make([][]rune, 0, len(g.texts))
	for _, buffer := range g.texts {
		if len(buffer) < g.opts.NodeRunes {
			continue
		}
		window := append([]rune(nil), buffer[len(buffer)-g.opts.NodeRunes:]...)
		windows = append(windows, window)
	}
	g.mu.Unlock()

	for _, window := range windows {
		if blockRepeats(window, g.opts) {
			g.hitFlag.Store(true)
			g.cancel()
			return
		}
	}
}

// Hit 本次输出是否因重复被中断
func (g *Guard) Hit() bool {
	return g != nil && g.hitFlag.Load()
}

// Detected 静态判定：文本尾部最后一个节点是否属于持续复读（非流式响应也用它）。
func Detected(text string, opts Options) bool {
	opts = opts.normalize()
	runes := []rune(text)
	if len(runes) < opts.NodeRunes {
		return false
	}
	return blockRepeats(runes[len(runes)-opts.NodeRunes:], opts)
}

// blockRepeats 把节点切成若干等长片段，判断相邻片段的平均相似度是否超过阈值。
func blockRepeats(window []rune, opts Options) bool {
	segments := opts.NodeRunes / opts.SegmentRunes
	if segments < 2 || len(window) < opts.NodeRunes {
		return false
	}
	previous := normalizeForLoop(window[:opts.SegmentRunes])
	sum := 0.0
	for index := 1; index < segments; index++ {
		current := normalizeForLoop(window[index*opts.SegmentRunes : (index+1)*opts.SegmentRunes])
		sum += Dice(string(previous), string(current))
		previous = current
	}
	return sum/float64(segments-1) > opts.Similarity
}

// normalizeForLoop 去掉空白、数字、标点与符号后再比较：
// 逐帧描述里「Frame9 / Frame10 / Frame11」只有编号不同，内容其实一致；
// 不归一化会让这类正常枚举看起来各不相同，反而漏掉真正的逐帧复读。
func normalizeForLoop(runes []rune) []rune {
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if unicode.IsSpace(r) || unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		out = append(out, r)
	}
	return out
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
