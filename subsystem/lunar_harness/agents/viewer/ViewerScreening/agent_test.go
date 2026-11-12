package ViewerScreening

// agent_test.go 钉住 submit_impression 的参数校验。

import (
	"strings"
	"testing"
)

// TestParseImpressionArgs 正常参数会被去空白并透传。
func TestParseImpressionArgs(t *testing.T) {
	impression, err := parseImpressionArgs(`{"summary":" 一段旅行记录 ","topics":["旅行","Vlog"],"tone":"轻松"}`)
	if err != nil {
		t.Fatalf("正常参数不应报错: %v", err)
	}
	if impression.Summary != "一段旅行记录" || impression.Tone != "轻松" {
		t.Fatalf("字段应去空白: %+v", impression)
	}
	if len(impression.Topics) != 2 || impression.Topics[0] != "旅行" {
		t.Fatalf("主题词不符: %+v", impression.Topics)
	}
}

// TestParseImpressionArgsMissing 摘要/主题/氛围任一缺失都报错。
func TestParseImpressionArgsMissing(t *testing.T) {
	if _, err := parseImpressionArgs(`{"topics":["a","b"],"tone":"轻松"}`); err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("缺少 summary 应报错: %v", err)
	}
	if _, err := parseImpressionArgs(`{"summary":"摘要","topics":[],"tone":"轻松"}`); err == nil || !strings.Contains(err.Error(), "主题词") {
		t.Fatalf("主题词为空应报错: %v", err)
	}
	if _, err := parseImpressionArgs(`{"summary":"摘要","topics":["a","b"],"tone":"  "}`); err == nil || !strings.Contains(err.Error(), "tone") {
		t.Fatalf("缺少 tone 应报错: %v", err)
	}
}

// TestParseImpressionArgsInvalid 主题词数量越界或 JSON 非法都报错。
func TestParseImpressionArgsInvalid(t *testing.T) {
	if _, err := parseImpressionArgs(`{"summary":"摘要","topics":["只有一"],"tone":"轻松"}`); err == nil {
		t.Fatal("主题词少于 2 个应报错")
	}
	if _, err := parseImpressionArgs(`{"summary":"摘要","topics":["1","2","3","4","5","6"],"tone":"轻松"}`); err == nil {
		t.Fatal("主题词多于 5 个应报错")
	}
	if _, err := parseImpressionArgs("不是 JSON"); err == nil {
		t.Fatal("非 JSON 参数应报错")
	}
}
