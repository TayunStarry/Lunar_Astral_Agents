package ViewerRemark

// agent_test.go 钉住 submit_remark 的参数校验与候选清单渲染：
// 每条评论各自用名字或序号指定 @ 目标，任何非法参数都直接报错。

import (
	"strings"
	"testing"
)

// TestParseRemarkArgsPerComment 两条评论各自带 @ 决策，互不影响。
func TestParseRemarkArgsPerComment(t *testing.T) {
	remark, err := parseRemarkArgs(`{"comments":[{"text":"这条太治愈了","mention":0,"at":[]},{"text":"求个歌单","mention":2,"at":["张三",2]}]}`)
	if err != nil {
		t.Fatalf("正常参数不应报错: %v", err)
	}
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

// TestParseRemarkArgsInvalid 缺少字段、mention 越界、at 越界都报错，不做静默截断。
func TestParseRemarkArgsInvalid(t *testing.T) {
	cases := map[string]string{
		"缺 comments": `{}`,
		"空 comments": `{"comments":[]}`,
		"缺 text":     `{"comments":[{"mention":0}]}`,
		"缺 mention":  `{"comments":[{"text":"a"}]}`,
		"mention 越界": `{"comments":[{"text":"a","mention":4}]}`,
		"at 越界":      `{"comments":[{"text":"a","mention":3,"at":["甲","乙","丙","丁"]}]}`,
		"at 无法识别":    `{"comments":[{"text":"a","mention":1,"at":[true]}]}`,
		"评论过多":       `{"comments":[{"text":"a","mention":0},{"text":"b","mention":0},{"text":"c","mention":0}]}`,
		"非 JSON":     `不是 JSON`,
	}
	for name, raw := range cases {
		if _, err := parseRemarkArgs(raw); err == nil {
			t.Fatalf("%s 应报错，实际通过了: %s", name, raw)
		}
	}
}

// TestParseRemarkArgsTrims 正文去空白并按上限截断，仍保留自己的 @ 决策。
func TestParseRemarkArgsTrims(t *testing.T) {
	remark, err := parseRemarkArgs(`{"comments":[{"text":"  太真实了  ","mention":1,"at":["张三"]}]}`)
	if err != nil {
		t.Fatalf("正常参数不应报错: %v", err)
	}
	if len(remark.Comments) != 1 || remark.Comments[0].Text != "太真实了" {
		t.Fatalf("正文应去空白: %+v", remark.Comments)
	}
	if remark.Comments[0].Mention != 1 || len(remark.Comments[0].At) != 1 {
		t.Fatalf("@ 决策应保留: %+v", remark.Comments[0])
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
