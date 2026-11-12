package ViewerCensor

// agent_test.go 钉住 submit_review 的参数校验。

import (
	"strings"
	"testing"
)

// TestParseReviewArgs 正常通过/未通过都能解析。
func TestParseReviewArgs(t *testing.T) {
	pass, err := parseReviewArgs(`{"pass":true,"reason":""}`)
	if err != nil || !pass.Pass || pass.Reason != "" {
		t.Fatalf("通过结果不符: %+v / %v", pass, err)
	}
	blocked, err := parseReviewArgs(`{"pass":false,"reason":" 色情低俗 "}`)
	if err != nil || blocked.Pass || blocked.Reason != "色情低俗" {
		t.Fatalf("未通过结果不符: %+v / %v", blocked, err)
	}
	// 通过时即使模型带了原因也清空，保持「通过无原因」的既有语义
	pass, err = parseReviewArgs(`{"pass":true,"reason":"没什么问题"}`)
	if err != nil || pass.Reason != "" {
		t.Fatalf("通过时应清空原因: %+v / %v", pass, err)
	}
}

// TestParseReviewArgsMissing 缺少 pass 或未通过原因都报错。
func TestParseReviewArgsMissing(t *testing.T) {
	if _, err := parseReviewArgs(`{"reason":""}`); err == nil || !strings.Contains(err.Error(), "pass") {
		t.Fatalf("缺少 pass 应报错: %v", err)
	}
	if _, err := parseReviewArgs(`{"pass":false,"reason":"  "}`); err == nil || !strings.Contains(err.Error(), "原因") {
		t.Fatalf("未通过缺原因应报错: %v", err)
	}
}

// TestParseReviewArgsInvalid 参数非法时报错，不静默退化成通过。
func TestParseReviewArgsInvalid(t *testing.T) {
	if _, err := parseReviewArgs(`{"pass":"yes","reason":""}`); err == nil {
		t.Fatal("pass 类型非法应报错")
	}
	if _, err := parseReviewArgs("不是 JSON"); err == nil {
		t.Fatal("非 JSON 参数应报错")
	}
}
