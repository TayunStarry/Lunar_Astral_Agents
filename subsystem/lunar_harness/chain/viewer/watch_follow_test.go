package viewer

// watch_follow_test.go 收束评论 @ 特别关注：人数范围 1-3、去重、候选不足与空列表降级。

import "testing"

// followPool 构造 n 个特别关注候选。
func followPool(n int) []MentionTarget {
	pool := make([]MentionTarget, 0, n)
	for i := 0; i < n; i++ {
		pool = append(pool, MentionTarget{Name: "关注" + string(rune('A'+i)), UID: int64(i + 1)})
	}
	return pool
}

// TestPickClosingMentionsRange 随机人数落在 1-3，且不重复 @ 同一个人。
func TestPickClosingMentionsRange(t *testing.T) {
	pool := followPool(10)
	for _, want := range []int{1, 2, 3} {
		fixed := want
		got := pickClosingMentions(pool, func(int) int { return fixed - 1 })
		if len(got) != want {
			t.Fatalf("期望 @ %d 人，实际 %d", want, len(got))
		}
		seen := make(map[int64]bool, len(got))
		for _, target := range got {
			if seen[target.UID] {
				t.Fatalf("同一个用户被 @ 了两次: %+v", got)
			}
			seen[target.UID] = true
		}
	}
}

// TestPickClosingMentionsCapsAtPoolSize 特别关注不足 3 人时取全部，不重复也不越界。
func TestPickClosingMentionsCapsAtPoolSize(t *testing.T) {
	got := pickClosingMentions(followPool(2), func(int) int { return 2 })
	if len(got) != 2 {
		t.Fatalf("候选只有 2 人时应全部 @，实际 %d", len(got))
	}
}

// TestPickClosingMentionsEmptyPoolDegrades 拿不到特别关注时降级为不 @。
func TestPickClosingMentionsEmptyPoolDegrades(t *testing.T) {
	if got := pickClosingMentions(nil, func(int) int { return 0 }); len(got) != 0 {
		t.Fatalf("空候选应降级为不 @，实际 %+v", got)
	}
	if got := pickClosingMentions([]MentionTarget{}, func(int) int { return 0 }); len(got) != 0 {
		t.Fatalf("空候选应降级为不 @，实际 %+v", got)
	}
}
