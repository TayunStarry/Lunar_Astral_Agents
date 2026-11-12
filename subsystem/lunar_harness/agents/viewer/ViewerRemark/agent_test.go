package ViewerRemark

// agent_test.go 评论智能体的解析与候选清单渲染：每条评论可各自用名字或序号指定 @ 目标。

import (
	"strings"
	"testing"
)

// TestParseRemarkPerComment 两条评论各自带 @ 决策，互不影响。
func TestParseRemarkPerComment(t *testing.T) {
	remark := parseRemark(`{"comments":[{"text":"这条太治愈了","mention":0,"at":[]},{"text":"求个歌单","mention":2,"at":["张三",2]}]}`)
	if len(remark.Comments) != 2 {
		t.Fatalf("应解析出 2 条评论: %+v", remark.Comments)
	}
	if remark.Comments[0].Text != "这条太治愈了" || remark.Comments[0].Mention != 0 || len(remark.Comments[0].At) != 0 {
		t.Fatalf("第一条应不 @: %+v", remark.Comments[0])
	}
	second := remark.Comments[1]
	if second.Mention != 2 || len(second.At) != 2 || second.At[0] != "张三" || second.At[1] != "2" {
		t.Fatalf("第二条应同时兼容名字与序号: %+v", second)
	}
}

// TestParseRemarkAtLimit 单条评论的 at 超过 3 个时只保留前 3 个。
func TestParseRemarkAtLimit(t *testing.T) {
	remark := parseRemark(`{"comments":[{"text":"a","mention":3,"at":["甲","乙","丙","丁"]}]}`)
	if len(remark.Comments) != 1 || len(remark.Comments[0].At) != mentionMax {
		t.Fatalf("at 应截到 %d 个，实际 %+v", mentionMax, remark.Comments)
	}
}

// TestParseRemarkStringItems 兼容纯字符串评论（视为不 @）。
func TestParseRemarkStringItems(t *testing.T) {
	remark := parseRemark(`{"comments":["第一条","第二条"]}`)
	if len(remark.Comments) != 2 {
		t.Fatalf("应解析出 2 条评论: %+v", remark.Comments)
	}
	for _, comment := range remark.Comments {
		if comment.Mention != 0 || len(comment.At) != 0 {
			t.Fatalf("字符串评论不应带 @: %+v", comment)
		}
	}
}

// TestParseRemarkPlainTextFallback 非 JSON 输出退回一条评论且不 @。
func TestParseRemarkPlainTextFallback(t *testing.T) {
	remark := parseRemark("这段剪辑太丝滑了")
	if len(remark.Comments) != 1 || remark.Comments[0].Text != "这段剪辑太丝滑了" {
		t.Fatalf("纯文本兜底不符: %+v", remark)
	}
	if remark.Comments[0].Mention != 0 || len(remark.Comments[0].At) != 0 {
		t.Fatalf("纯文本兜底不应 @: %+v", remark.Comments[0])
	}
}

// TestWithCandidates 候选清单只暴露序号与展示名；为空时明确要求不指定人。
func TestWithCandidates(t *testing.T) {
	material := withCandidates("视频印象", []MentionCandidate{{Index: 1, Name: "张三"}, {Index: 2, Name: "李四"}})
	if !strings.Contains(material, "1. 张三") || !strings.Contains(material, "2. 李四") {
		t.Fatalf("候选清单渲染不符: %s", material)
	}
	empty := withCandidates("视频印象", nil)
	if !strings.Contains(empty, "mention 请填 0") {
		t.Fatalf("无候选时应要求不 @: %s", empty)
	}
}
