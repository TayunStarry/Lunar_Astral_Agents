package DocumentReader

// scan_test.go 轻量扫描通道的纯函数单元测试：门槛判断、计划解析、选片与笔记渲染。
// 只测试不依赖真实模型的分支，不发起任何网络请求。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// withScanConfig 临时改写扫描配置，测试结束自动还原
func withScanConfig(t *testing.T, model string, swap *bool, threshold, maxChunks int) {
	t.Helper()
	oldModel := config.DocumentScanModel
	oldSwap := config.DocumentScanSwap
	oldThreshold := config.DocumentScanThreshold
	oldMaxChunks := config.DocumentScanMaxChunks
	t.Cleanup(func() {
		config.DocumentScanModel = oldModel
		config.DocumentScanSwap = oldSwap
		config.DocumentScanThreshold = oldThreshold
		config.DocumentScanMaxChunks = oldMaxChunks
	})
	config.DocumentScanModel = model
	config.DocumentScanSwap = swap
	config.DocumentScanThreshold = threshold
	config.DocumentScanMaxChunks = maxChunks
}

// fakeChunks 生成指定片数的分片计划
func fakeChunks(count int) []chunkSpan {
	spans := make([]chunkSpan, count)
	for i := 0; i < count; i++ {
		spans[i] = chunkSpan{Index: i, StartLine: i*100 + 1, EndLine: (i + 1) * 100, Chars: 9000}
	}
	return spans
}

// TestShouldScan 只有大文件、片数足够且配置开启时才切换模型。
func TestShouldScan(t *testing.T) {
	enabled := true
	disabled := false

	withScanConfig(t, "light_reasoning", &enabled, 50000, 40)
	big := &docContext{Profile: formatProfile{Runes: 60000}, Chunks: fakeChunks(6)}
	if !shouldScan(big) {
		t.Fatal("大文件 + 片数充足时应启用扫描")
	}

	cases := []struct {
		name string
		doc  *docContext
	}{
		{"二进制", &docContext{Profile: formatProfile{Runes: 60000, Binary: true}, Chunks: fakeChunks(6)}},
		{"未达阈值", &docContext{Profile: formatProfile{Runes: 40000}, Chunks: fakeChunks(6)}},
		{"片数不足", &docContext{Profile: formatProfile{Runes: 60000}, Chunks: fakeChunks(4)}},
	}
	for _, tc := range cases {
		if shouldScan(tc.doc) {
			t.Fatalf("%s 不应启用扫描", tc.name)
		}
	}

	withScanConfig(t, "light_reasoning", &disabled, 50000, 40)
	if shouldScan(big) {
		t.Fatal("配置关闭模型切换时不应启用扫描")
	}

	withScanConfig(t, "  ", &enabled, 50000, 40)
	if shouldScan(big) {
		t.Fatal("扫描模型为空时不应启用扫描")
	}
}

// TestParseScanPlan 计划解析必须容忍代码围栏与非法输入。
func TestParseScanPlan(t *testing.T) {
	plan := parseScanPlan("```json\n{\"mode\":\"generic\",\"goal\":\"总结全文结构\",\"focus\":[\"章节\",\"章节\",\"  \"]}\n```", "随便看看")
	if plan.Mode != scanModeGeneric {
		t.Fatalf("模式解析错误: %q", plan.Mode)
	}
	if plan.Goal != "总结全文结构" {
		t.Fatalf("目标解析错误: %q", plan.Goal)
	}
	if len(plan.Focus) != 1 || plan.Focus[0] != "章节" {
		t.Fatalf("关注词应去重去空，实际 %+v", plan.Focus)
	}

	plan = parseScanPlan("{\"mode\":\"direct\"}", "逐行核对")
	if plan.Mode != scanModeDirect {
		t.Fatalf("direct 未识别: %+v", plan)
	}
	if plan.Goal != "逐行核对" {
		t.Fatalf("目标为空时应回退需求，实际 %q", plan.Goal)
	}

	plan = parseScanPlan("我觉得应该整体概括一下", "讲讲结论")
	if plan.Mode != scanModeTargeted {
		t.Fatalf("非 JSON 应退化为定向扫描，实际 %q", plan.Mode)
	}
	if !strings.Contains(plan.Goal, "整体概括") {
		t.Fatalf("非 JSON 时目标应为原文，实际 %q", plan.Goal)
	}

	plan = parseScanPlan("", "")
	if plan.Mode != scanModeTargeted || plan.Goal != "" {
		t.Fatalf("空输入回退异常: %+v", plan)
	}
}

// TestSelectScanSpans 未超上限时全取；超限时保留首尾并优先高相关片。
func TestSelectScanSpans(t *testing.T) {
	doc := &docContext{Chunks: fakeChunks(10)}
	if spans := selectScanSpans(doc, doc.Chunks, scanPlan{}, 0); len(spans) != 0 {
		t.Fatalf("上限为 0 时应返回空，实际 %d", len(spans))
	}
	if spans := selectScanSpans(doc, doc.Chunks, scanPlan{}, 20); len(spans) != 10 {
		t.Fatalf("未超上限应全取，实际 %d", len(spans))
	}

	lines := make([]string, 1000)
	for i := range lines {
		lines[i] = "普通内容"
	}
	lines[450] = "认证令牌 认证令牌"
	doc = &docContext{Chunks: fakeChunks(10), Lines: lines}

	spans := selectScanSpans(doc, doc.Chunks, scanPlan{Goal: "认证令牌"}, 4)
	if len(spans) != 4 {
		t.Fatalf("应选出 4 片，实际 %d", len(spans))
	}
	if spans[0].Index != 0 || spans[len(spans)-1].Index != 9 {
		t.Fatalf("应保留首尾分片，实际 %+v", spans)
	}
	hasHit := false
	for _, span := range spans {
		if span.Index == 4 {
			hasHit = true
		}
		if span.StartLine > span.EndLine {
			t.Fatalf("片区间非法: %+v", span)
		}
	}
	if !hasHit {
		t.Fatalf("命中的第 5 片应被选中，实际 %+v", spans)
	}
	for i := 1; i < len(spans); i++ {
		if spans[i-1].Index >= spans[i].Index {
			t.Fatalf("选片应按片序升序: %+v", spans)
		}
	}

	sampledDoc := &docContext{Chunks: fakeChunks(10)}
	sampled := selectScanSpans(sampledDoc, sampledDoc.Chunks, scanPlan{Goal: "完全不存在的词"}, 3)
	if len(sampled) != 3 {
		t.Fatalf("无命中时应均匀抽样 3 片，实际 %d", len(sampled))
	}
	for i := 1; i < len(sampled); i++ {
		if sampled[i-1].Index >= sampled[i].Index {
			t.Fatalf("抽样应按片序升序: %+v", sampled)
		}
	}
}

// TestScanTerms 中文长串必须补二元/三元词，并做去重。
func TestScanTerms(t *testing.T) {
	terms := scanTerms("查找用户认证令牌机制", []string{"令牌", "令牌"})
	if len(terms) == 0 {
		t.Fatal("检索词不应为空")
	}
	seen := map[string]bool{}
	for _, term := range terms {
		if seen[term] {
			t.Fatalf("检索词重复: %q", term)
		}
		seen[term] = true
		if len([]rune(term)) < 2 {
			t.Fatalf("单字不应成为检索词: %q", term)
		}
	}
	if !seen["认证令牌"] && !seen["令牌机"] {
		t.Fatalf("中文长串应产生三元词，实际 %+v", terms)
	}
	if len(terms) > 80 {
		t.Fatalf("检索词超过上限: %d", len(terms))
	}
}

// TestRenderScanNotes 单片与总量都要截断，空输入给出占位。
func TestRenderScanNotes(t *testing.T) {
	if out := renderScanNotes(nil); out != "（无扫描笔记）" {
		t.Fatalf("空笔记占位错误: %q", out)
	}

	long := strings.Repeat("字", scanNoteMaxChars+200)
	notes := []scanNote{
		{Span: chunkSpan{Index: 0, StartLine: 1, EndLine: 100}, Text: long},
		{Span: chunkSpan{Index: 1, StartLine: 101, EndLine: 200}, Text: "短笔记"},
	}
	out := renderScanNotes(notes)
	if !strings.Contains(out, "◆ 片 #0 L1-100") {
		t.Fatalf("笔记缺少片头: %q", out)
	}
	if len([]rune(out)) > scanMergeNotesChars+128 {
		t.Fatalf("笔记总量超限: %d", len([]rune(out)))
	}
}

// TestScanChunksByConfig 扫描单片上限与默认不同时，必须按配置重建分批而不是硬截断。
func TestScanChunksByConfig(t *testing.T) {
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = strings.Repeat("字", 50)
	}
	doc := &docContext{Lines: lines, Chunks: buildChunkPlan(lines)}

	old := config.DocumentScanChunkRunes
	t.Cleanup(func() { config.DocumentScanChunkRunes = old })

	config.DocumentScanChunkRunes = 4000
	small := scanChunks(doc)
	if len(small) <= len(doc.Chunks) {
		t.Fatalf("单片上限调小后应切出更多片，实际 %d", len(small))
	}
	for _, span := range small {
		if span.Chars > 4000 {
			t.Fatalf("第 %d 片 %d 字，超过配置上限 4000", span.Index, span.Chars)
		}
	}

	config.DocumentScanChunkRunes = chunkRunes
	if got := scanChunks(doc); len(got) != len(doc.Chunks) {
		t.Fatalf("配置与默认一致时应复用既有分批，实际 %d", len(got))
	}
}

// TestNotesFallback 归并失败时的兜底必须带说明与行区间，并遵守总量上限。
func TestNotesFallback(t *testing.T) {
	notes := []scanNote{
		{Span: chunkSpan{Index: 0, StartLine: 1, EndLine: 100}, Text: strings.Repeat("字", scanNoteMaxChars)},
		{Span: chunkSpan{Index: 1, StartLine: 101, EndLine: 200}, Text: strings.Repeat("字", scanNoteMaxChars)},
		{Span: chunkSpan{Index: 2, StartLine: 201, EndLine: 300}, Text: strings.Repeat("字", scanNoteMaxChars)},
	}
	out := notesFallback(notes)
	if !strings.HasPrefix(out, "（轻量扫描笔记汇总") {
		t.Fatalf("兜底缺少说明: %q", out[:20])
	}
	if !strings.Contains(out, "【片 #0 L1-100】") {
		t.Fatalf("兜底缺少行区间: %q", out[:60])
	}
	if len([]rune(out)) > scanFallbackChars+256 {
		t.Fatalf("兜底总量超限: %d", len([]rune(out)))
	}
}
