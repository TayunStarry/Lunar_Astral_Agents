package viewer

// watch_follow_test.go 评论 @ 特别关注：每条评论各自给人数（0-3）与可选名字/序号，
// 链侧逐条校验、去重、候选不足与空列表降级。

import "testing"

// followPool 构造 n 个特别关注候选。
func followPool(n int) []MentionTarget {
	pool := make([]MentionTarget, 0, n)
	for i := 0; i < n; i++ {
		pool = append(pool, MentionTarget{Name: "关注" + string(rune('A'+i)), UID: int64(i + 1)})
	}
	return pool
}

// resolve 固定随机源为 0，便于断言回退挑选的稳定结果。
func resolve(pool []MentionTarget, mention int, at []string) []MentionTarget {
	return resolveMentions(pool, mention, at, func(int) int { return 0 })
}

// TestPickMentionsRange 模型要求几人就挑几人，且不重复 @ 同一个人。
func TestPickMentionsRange(t *testing.T) {
	pool := followPool(10)
	for _, want := range []int{1, 2, 3} {
		got := pickMentions(pool, want, func(int) int { return 0 })
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

// TestPickMentionsCapsAtPoolSize 特别关注不足时取全部，不重复也不越界。
func TestPickMentionsCapsAtPoolSize(t *testing.T) {
	got := pickMentions(followPool(2), 3, func(int) int { return 1 })
	if len(got) != 2 {
		t.Fatalf("候选只有 2 人时应全部 @，实际 %d", len(got))
	}
}

// TestPickMentionsClampsOverMax 模型给的人数超过上限时钳制到 3。
func TestPickMentionsClampsOverMax(t *testing.T) {
	got := pickMentions(followPool(10), 9, func(int) int { return 0 })
	if len(got) != closingMentionMax {
		t.Fatalf("超上限应钳制到 %d 人，实际 %d", closingMentionMax, len(got))
	}
}

// TestPickMentionsZeroOrEmpty 模型给 0 或候选为空时都降级为不 @。
func TestPickMentionsZeroOrEmpty(t *testing.T) {
	if got := pickMentions(nil, 2, func(int) int { return 0 }); len(got) != 0 {
		t.Fatalf("空候选应降级为不 @，实际 %+v", got)
	}
	if got := pickMentions([]MentionTarget{}, 2, func(int) int { return 0 }); len(got) != 0 {
		t.Fatalf("空候选应降级为不 @，实际 %+v", got)
	}
	if got := pickMentions(followPool(3), 0, func(int) int { return 0 }); len(got) != 0 {
		t.Fatalf("模型给 0 时不应 @，实际 %+v", got)
	}
}

// TestResolveMentionsNamedUser 模型指定合法名字时，按清单原样 @ 该用户，不随机替换。
func TestResolveMentionsNamedUser(t *testing.T) {
	pool := followPool(5)
	got := resolve(pool, 3, []string{"关注D"})
	if len(got) != 1 || got[0].UID != pool[3].UID {
		t.Fatalf("应只 @ 模型点名的关注D，实际 %+v", got)
	}
}

// TestResolveMentionsIndex 模型指定候选序号（1 基）时，映射到清单里的对应用户。
func TestResolveMentionsIndex(t *testing.T) {
	pool := followPool(5)
	got := resolve(pool, 1, []string{"2"})
	if len(got) != 1 || got[0].UID != pool[1].UID {
		t.Fatalf("应按序号 2 命中第二个候选，实际 %+v", got)
	}
}

// TestResolveMentionsUnknownFallsBack 清单外的名字直接丢弃；全部无效时回退随机挑选。
func TestResolveMentionsUnknownFallsBack(t *testing.T) {
	pool := followPool(6)
	got := resolve(pool, 2, []string{"幽灵用户"})
	if len(got) != 2 {
		t.Fatalf("全部非法时应按人数随机挑 2 人，实际 %+v", got)
	}
	for _, target := range got {
		if target.Name == "幽灵用户" {
			t.Fatalf("不允许把模型编造的名字当成 @ 目标: %+v", got)
		}
	}
}

// TestResolveMentionsPartialInvalid 部分合法时保留合法项，不补足到模型给的人数。
func TestResolveMentionsPartialInvalid(t *testing.T) {
	pool := followPool(5)
	got := resolve(pool, 2, []string{"关注A", "查无此人"})
	if len(got) != 1 || got[0].UID != pool[0].UID {
		t.Fatalf("应只保留能认出的关注A，实际 %+v", got)
	}
}

// TestResolveMentionsZero 模型只给 0、没给名字时完全不 @。
func TestResolveMentionsZero(t *testing.T) {
	if got := resolve(followPool(5), 0, nil); len(got) != 0 {
		t.Fatalf("人数 0 时应不 @，实际 %+v", got)
	}
}

// TestResolveMentionsZeroOverridesNames 人数为 0 时即使给了名字也不 @（0 是明确的不打扰）。
func TestResolveMentionsZeroOverridesNames(t *testing.T) {
	if got := resolve(followPool(5), 0, []string{"关注A"}); len(got) != 0 {
		t.Fatalf("人数 0 应压过名字，实际 %+v", got)
	}
}

// TestResolveMentionsEmptyPool 没有可选的特别关注时，任何指定都无效。
func TestResolveMentionsEmptyPool(t *testing.T) {
	if got := resolve(nil, 2, []string{"关注A"}); len(got) != 0 {
		t.Fatalf("候选为空时应不 @，实际 %+v", got)
	}
}

// TestResolveMentionsClampsNamed 模型点名超过 3 个时只取前 3 个，且去重。
func TestResolveMentionsClampsNamed(t *testing.T) {
	pool := followPool(6)
	got := resolve(pool, 3, []string{"关注A", "关注B", "关注A", "关注C", "关注D"})
	if len(got) != closingMentionMax {
		t.Fatalf("点名超过上限时应只取 %d 人，实际 %+v", closingMentionMax, got)
	}
	if got[0].UID == got[1].UID || got[1].UID == got[2].UID {
		t.Fatalf("不应重复 @ 同一个人: %+v", got)
	}
}
