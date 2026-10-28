package WebSearch

import (
	"strings"
	"testing"
)

func TestIsNoRelevanceDigest(t *testing.T) {
	cases := []string{
		"（未检索到与问题直接相关的信息）",
		"未检索到相关信息。",
		"经过检查，没有找到与问题直接相关的信息。",
	}
	for _, text := range cases {
		if !isNoRelevanceDigest(text) {
			t.Fatalf("应识别为否定占位句: %q", text)
		}
	}
	if isNoRelevanceDigest("这个页面介绍了 Qwen 模型的参数与训练数据。") {
		t.Fatal("正常页面摘要不应被误判")
	}
}

func TestPageContentDigestUsesSourceText(t *testing.T) {
	sources := []digestSource{
		{Label: "详情页 1", Title: "页面 A", Text: "页面 A 的主要内容是模型参数与部署方式。"},
	}
	out := pageContentDigest(sources)
	if strings.Contains(out, "未检索到") {
		t.Fatalf("不应输出否定占位句: %q", out)
	}
	if !strings.Contains(out, "页面 A 的主要内容") {
		t.Fatalf("应输出页面正文摘要: %q", out)
	}
}

func TestComposeReportReplacesNoRelevancePlaceholder(t *testing.T) {
	in := reportInput{
		Digest: "（未检索到与问题直接相关的信息）",
		Pages: []PageCapture{
			{Title: "页面 A", Domain: "example.com", Text: "页面正文介绍了模型参数和训练数据。", TextChars: 18},
		},
	}
	out := composeReport(in)
	if strings.Contains(out, "未检索到与问题直接相关的信息") {
		t.Fatalf("输出仍包含否定占位句: %q", out)
	}
	if !strings.Contains(out, "页面正文介绍了模型参数") {
		t.Fatalf("输出应包含页面内容摘要: %q", out)
	}
}
