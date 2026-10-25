package loopguard

import (
	"strings"
	"testing"
)

// fixedWidth 把一段文本重复/截断到恰好 w 个字符，便于构造等长片段。
func fixedWidth(text string, w int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return strings.Repeat("。", w)
	}
	for len(runes) < w {
		runes = append(runes, []rune(text)...)
	}
	return string(runes[:w])
}

// distinctSegments 返回 n 个互不相同的 128 字片段（模拟正常长文）。
func distinctSegments(n int) []string {
	base := []string{
		"第一段说的是游戏关服之后运营方反复道歉，评论区都在调侃那张养老保险月卡。",
		"画面随后切到街边小店，人来人往，镜头缓慢推近招牌上的手写字。",
		"接着是一段访谈录音，嘉宾提到了自己刚开始做视频时的窘迫经历。",
		"视频中段插入了几张数据图表，主持人逐条解释增长曲线背后的原因。",
		"后半段换成第一视角的旅行记录，海边风很大，远处有人在放风筝。",
		"评论区讨论最多的是剪辑节奏，有人觉得太快，也有人就喜欢这种密度。",
		"片尾字幕列出了参与制作的人员名单，背景音乐慢慢淡出。",
		"最后一段是作者的自述，他说做这个系列只是想记录普通人的日常。",
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fixedWidth(base[i%len(base)], 128))
	}
	return out
}

// TestDetectedNormalLongText 正常长文（八个不同片段）不应被误判。
func TestDetectedNormalLongText(t *testing.T) {
	text := strings.Join(distinctSegments(8), "")
	if Detected(text, DefaultOptions()) {
		t.Fatal("正常长文不应被判定为重复输出")
	}
}

// TestDetectedShortTemplateBurst 短模板枚举（末尾 3 段 Same as Frame8）不应误判：
// 平均相似度会被前面正常内容稀释，这是节点+平均策略的核心取舍。
func TestDetectedShortTemplateBurst(t *testing.T) {
	segments := distinctSegments(5)
	same := fixedWidth("Same as Frame8.", 128)
	text := strings.Join(append(segments, same, same, same), "")
	if Detected(text, DefaultOptions()) {
		t.Fatal("短时间的模板化枚举不应被判为持续复读")
	}
}

// TestDetectedSustainedLoop 整个节点都在复读时必须命中。
func TestDetectedSustainedLoop(t *testing.T) {
	unit := fixedWidth("Frame520: the character looks at the camera while subtitles keep changing.", 128)
	text := strings.Repeat(unit, 8)
	if !Detected(text, DefaultOptions()) {
		t.Fatal("持续的逐帧复读应被判定为重复输出")
	}
}

// TestDetectedThresholdOption 平均相似度阈值生效：七段相同、一段不同时
// 平均约 0.86，0.65 命中、0.90 不命中。
func TestDetectedThresholdOption(t *testing.T) {
	same := fixedWidth("同一段正在反复输出的描述文字。", 128)
	text := fixedWidth("完全不同的开头，只出现一次。", 128) + strings.Repeat(same, 7)
	if !Detected(text, DefaultOptions()) {
		t.Fatal("默认阈值应命中高平均相似度")
	}
	strict := Options{NodeRunes: 1024, SegmentRunes: 128, Similarity: 0.90}
	if Detected(text, strict) {
		t.Fatal("提高阈值后不应再命中")
	}
}

// TestGuardChecksAtNodes 守卫按节点检查：不足 1024 字不判定，跨过节点才判定。
func TestGuardChecksAtNodes(t *testing.T) {
	hit := false
	guard := New(func() { hit = true }, DefaultOptions())
	unit := fixedWidth("重复输出同一段内容。", 128)
	text := strings.Repeat(unit, 8)

	guard.Start()
	guard.Feed(KindThink, text[:1000])
	if hit {
		t.Fatal("不足一个节点时不应判定")
	}
	guard.Feed(KindThink, text[1000:])
	if !hit {
		t.Fatal("跨过 1024 字节点后应判定重复")
	}
	if !guard.Hit() {
		t.Fatal("命中标记未置位")
	}
}

// TestGuardStreamsIndependent 思考与正文独立计数：正文正常不应被思考的复读牵连。
func TestGuardStreamsIndependent(t *testing.T) {
	hit := false
	guard := New(func() { hit = true }, DefaultOptions())
	normal := strings.Join(distinctSegments(8), "")
	guard.Feed(KindContent, normal)
	if hit {
		t.Fatal("正常正文不应命中")
	}
	unit := fixedWidth("重复输出同一段内容。", 128)
	guard.Feed(KindThink, strings.Repeat(unit, 8))
	if !hit {
		t.Fatal("思考流持续复读应命中")
	}
}

// TestGuardStopChecksTail 流在节点中间结束时，Stop 对最后一个节点长度做兜底检查。
func TestGuardStopChecksTail(t *testing.T) {
	hit := false
	guard := New(func() { hit = true }, DefaultOptions())
	unit := fixedWidth("重复输出同一段内容。", 128)
	// 直接构造缓冲区并模拟「节点尚未检查」：对应流在节点中间卡住并正常收尾
	guard.texts[KindThink] = []rune(strings.Repeat(unit, 8))
	guard.checked[KindThink] = 0
	guard.Stop()
	if !hit {
		t.Fatal("Stop 应对末尾持续复读做兜底判定")
	}
}
