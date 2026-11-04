package SearchSummarizer

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

func TestIsNoRelevanceExport(t *testing.T) {
	if !IsNoRelevance("（未检索到与问题直接相关的信息）") {
		t.Fatal("导出包装应复用同一判定")
	}
}

func TestPageContentDigestUsesSourceText(t *testing.T) {
	sources := []Source{
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
