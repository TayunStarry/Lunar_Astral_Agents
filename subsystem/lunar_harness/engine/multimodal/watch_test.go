package multimodal

// watch_test.go 统一观看管线：切片规则的回归测试
//
// 钉住三条可预测性保证：
//   1. 视频恒按 MediaChunkSeconds 切片，恰为整片时长时不产生空尾片
//   2. 不足 1 秒的尾巴不作为独立切片（下载截断常见）
//   3. 切片数受 maxSlices 限制

import "testing"

// TestSplitMediaSegments 切片边界：60 秒整、含尾巴、超上限三种情形
func TestSplitMediaSegments(t *testing.T) {
	exact := SplitMediaSegments(120, 0)
	if len(exact) != 2 || exact[1].Start != 60 || exact[1].End != 120 {
		t.Fatalf("120 秒应切成 2 片且第二片为 60-120，实得 %+v", exact)
	}

	tail := SplitMediaSegments(120.4, 0)
	if len(tail) != 2 {
		t.Fatalf("0.4 秒尾巴不应单独成片，实得 %d 片", len(tail))
	}

	capped := SplitMediaSegments(600, 3)
	if len(capped) != 3 || capped[2].End != 180 {
		t.Fatalf("应受 maxSlices 限制为 3 片且末片到 180 秒，实得 %+v", capped)
	}

	if got := SplitMediaSegments(30, 0); len(got) != 1 || got[0].End != 30 {
		t.Fatalf("30 秒视频应为单切片 0-30，实得 %+v", got)
	}
}

// TestPickSegments 长视频超上限时等距选片：覆盖首尾整段，而不是只看开头
func TestPickSegments(t *testing.T) {
	all := SplitMediaSegments(600, 0)
	if len(all) != 10 {
		t.Fatalf("600 秒应切成 10 片，实得 %d", len(all))
	}
	picked := pickSegments(all, 4)
	if len(picked) != 4 {
		t.Fatalf("应选出 4 片，实得 %d", len(picked))
	}
	if picked[0].Start != 0 || picked[3].End != 600 {
		t.Fatalf("选片应覆盖首尾，实得 %+v", picked)
	}
	if short := pickSegments(all, 20); len(short) != len(all) {
		t.Fatalf("未超上限时应原样返回，实得 %d", len(short))
	}
}
