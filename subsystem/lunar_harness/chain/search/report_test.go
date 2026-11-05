package search

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/chain/search/crawler"
)

func TestComposeReportReplacesNoRelevancePlaceholder(t *testing.T) {
	in := reportInput{
		Digest: "（未检索到与问题直接相关的信息）",
		Pages: []crawler.PageCapture{
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

func TestComposeReportPrefersDigest(t *testing.T) {
	in := reportInput{Digest: "页面介绍了模型参数与部署方式。"}
	if out := composeReport(in); out != in.Digest {
		t.Fatalf("摘要有效时应直接采用摘要: %q", out)
	}
}
