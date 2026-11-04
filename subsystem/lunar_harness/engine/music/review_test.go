package music

// review_test.go 生成端体检的可枚举校验项测试。

import (
	"math"
	"testing"
)

// testBrief 16 小节、100 BPM、4/4：期望时长 38.4 秒。
func testBrief() Brief {
	return Brief{Title: "测试", Tempo: 100, Meter: "4/4", Bars: 16}
}

// buildNotes 构造 n 个三连音 token；step 为相邻起点间隔（10ms），pitch 为音高偏移。
func buildNotes(n, step, duration, pitch int) []int {
	ids := make([]int, 0, n*amtNotesPerStep)
	for index := 0; index < n; index++ {
		ids = append(ids,
			index*step,
			amtDurOffset+duration,
			amtNoteOffset+pitch+(index%12),
		)
	}
	return ids
}

func TestReviewTokensPassesRegularMelody(t *testing.T) {
	review := ReviewTokens(buildNotes(100, 38, 20, 60), testBrief())
	if len(review.Issues) != 0 {
		t.Fatalf("规整旋律不应有问题，实际 %v", review.Issues)
	}
	if review.Notes != 100 {
		t.Fatalf("音符数 = %d，期望 100", review.Notes)
	}
	if review.Score >= 1 {
		t.Fatalf("通过项排序分应接近 0，实际 %.3f", review.Score)
	}
}

func TestReviewTokensReportsNoNotes(t *testing.T) {
	review := ReviewTokens(nil, testBrief())
	if !containsIssue(review.Issues, issueNoNotes) {
		t.Fatalf("空输出应报 %s，实际 %v", issueNoNotes, review.Issues)
	}
	if !math.IsInf(review.Score, 1) {
		t.Fatalf("空输出排序分应为 +Inf，实际 %.3f", review.Score)
	}
}

func TestReviewTokensReportsMalformed(t *testing.T) {
	ids := buildNotes(50, 38, 20, 60)
	for index := 0; index < 10; index++ {
		ids = append(ids, 0, amtDurOffset+20, 99999)
	}
	review := ReviewTokens(ids, testBrief())
	if !containsIssue(review.Issues, issueMalformed) {
		t.Fatalf("损坏三元组应被检出，实际 %v", review.Issues)
	}
}

func TestReviewTokensReportsUnordered(t *testing.T) {
	ids := make([]int, 0, 60*amtNotesPerStep)
	for index := 59; index >= 0; index-- {
		ids = append(ids, index*38, amtDurOffset+20, amtNoteOffset+60)
	}
	review := ReviewTokens(ids, testBrief())
	if !containsIssue(review.Issues, issueUnordered) {
		t.Fatalf("乱序起始时间应被检出，实际 %v", review.Issues)
	}
}

func TestReviewTokensReportsNarrowPitch(t *testing.T) {
	ids := make([]int, 0, 60*amtNotesPerStep)
	for index := 0; index < 60; index++ {
		ids = append(ids, index*38, amtDurOffset+20, amtNoteOffset+60)
	}
	review := ReviewTokens(ids, testBrief())
	if !containsIssue(review.Issues, issueNarrowPitch) {
		t.Fatalf("单一音高应被检出，实际 %v", review.Issues)
	}
}

func TestReviewTokensReportsDenseTexture(t *testing.T) {
	ids := make([]int, 0, 40*amtNotesPerStep)
	for index := 0; index < 40; index++ {
		ids = append(ids, 0, amtDurOffset+100, amtNoteOffset+40+index%12)
	}
	review := ReviewTokens(ids, testBrief())
	if !containsIssue(review.Issues, issueDense) {
		t.Fatalf("同时发声过多应被检出，实际 %v", review.Issues)
	}
	if review.MaxOverlap < 40 {
		t.Fatalf("最大并发 = %d，期望 40", review.MaxOverlap)
	}
}

func TestReviewTokensReportsTooShort(t *testing.T) {
	review := ReviewTokens(buildNotes(20, 20, 10, 60), testBrief())
	if !containsIssue(review.Issues, issueTooShort) {
		t.Fatalf("篇幅过短应被检出，实际 %v", review.Issues)
	}
}

func TestDiagnosisIsBounded(t *testing.T) {
	review := Review{
		Issues: []string{issueDense, issueTooDense, issueNarrowPitch, issueSparse, issueTooShort},
	}
	hint := Diagnosis(review)
	if hint == "" {
		t.Fatal("存在问题码时应给出重写提示")
	}
	if len(hint) > reviewDiagnosisLimit {
		t.Fatalf("重写提示长度 %d 超过上限 %d", len(hint), reviewDiagnosisLimit)
	}
	if Diagnosis(Review{}) != "" {
		t.Fatal("无问题时不应追加重写提示")
	}
}

func TestClampDescriptionCollapsesLines(t *testing.T) {
	long := "第一行\n第二行\t  多余空白"
	if got := clampDescription(long); got != "第一行 第二行 多余空白" {
		t.Fatalf("单行化结果 = %q", got)
	}
	oversized := make([]rune, descriptionLimit+100)
	for index := range oversized {
		oversized[index] = 'a'
	}
	if got := clampDescription(string(oversized)); len([]rune(got)) != descriptionLimit {
		t.Fatalf("超长描述未截断：%d", len([]rune(got)))
	}
}
