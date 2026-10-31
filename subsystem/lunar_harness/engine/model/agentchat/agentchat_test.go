package agentchat

// agentchat_test.go 共享模型调用层的纯函数测试（不依赖模型服务）。

import "testing"

// TestRenderPromptSinglePass 占位符必须单次替换：插入值里的 {xxx} 不能被二次展开。
func TestRenderPromptSinglePass(t *testing.T) {
	template := "需求：{request}\n正文：{content}"
	got := RenderPrompt(template,
		"{request}", "看这段 {content}",
		"{content}", "真实正文",
	)
	want := "需求：看这段 {content}\n正文：真实正文"
	if got != want {
		t.Fatalf("占位符单次替换失败：\n得到 %q\n期望 %q", got, want)
	}
}

// TestStringOfParts 图文片段数组必须能还原为纯文本。
func TestStringOfParts(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "第一段"},
		map[string]any{"type": "image_url"},
		map[string]any{"type": "text", "text": "第二段"},
	}
	if got := StringOf(content); got != "第一段\n第二段" {
		t.Fatalf("片段还原失败: %q", got)
	}
	if got := StringOf("纯文本"); got != "纯文本" {
		t.Fatalf("字符串内容还原失败: %q", got)
	}
}

// TestHardCutRunes 超长文本按字符截断并保留省略号。
func TestHardCutRunes(t *testing.T) {
	text := "一二三四五"
	if got := HardCutRunes(text, 10); got != text {
		t.Fatalf("未超限不应截断: %q", got)
	}
	got := HardCutRunes(text, 3)
	if got != "一二…" {
		t.Fatalf("截断结果错误: %q", got)
	}
}
