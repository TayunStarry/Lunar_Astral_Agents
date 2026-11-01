package LunarPersonality

import "testing"

func TestSplitUserPrefixHandlesGroupPrefix(t *testing.T) {
	cases := []struct {
		text   string
		sender string
		body   string
	}{
		{"[用户: 钛宇星光阁]: 你好", "钛宇星光阁", "你好"},
		{"[群聊: 『 星月智能 』][用户: 月白清风]: 瞳瞳现在也就只能整整日语了", "月白清风", "瞳瞳现在也就只能整整日语了"},
		{"没有前缀的一句话", "", "没有前缀的一句话"},
	}
	for _, c := range cases {
		sender, body := splitUserPrefix(c.text)
		if sender != c.sender || body != c.body {
			t.Fatalf("拆分 %q 得到 (%q, %q)，期望 (%q, %q)", c.text, sender, body, c.sender, c.body)
		}
	}
}

func TestRecentAffectionRoundsPerRoleWindow(t *testing.T) {
	cacheMu.Lock()
	saved := cachedMessages
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		cachedMessages = saved
		cacheMu.Unlock()
	})

	records := []CachedMessage{
		{ID: "u1", Role: RoleUser, Content: "[群聊: 星月小屋][用户: 月白清风]: 你好"},
		{ID: "r1", Role: RoleAssistant, Content: "月白你好呀"},
	}
	for i := 0; i < 7; i++ {
		records = append(records,
			CachedMessage{ID: "u" + string(rune('a'+i)), Role: RoleUser, Content: "[用户: 钛宇星光阁]: 第" + string(rune('0'+i)) + "句话"},
			CachedMessage{ID: "r" + string(rune('a'+i)), Role: RoleAssistant, Content: "回应" + string(rune('0'+i))},
		)
	}
	// 没有回应的发言不构成成对交互
	records = append(records, CachedMessage{ID: "u-last", Role: RoleUser, Content: "[用户: 钛宇星光阁]: 还没回我"})

	cacheMu.Lock()
	cachedMessages = records
	cacheMu.Unlock()

	rounds := recentAffectionRounds()
	if len(rounds) != 1+affectionRoundWindow {
		t.Fatalf("应取月白 1 轮 + 钛宇 5 轮，实际 %d 轮", len(rounds))
	}
	if rounds[0].UserName != "月白清风" || rounds[0].ReplyText != "月白你好呀" {
		t.Fatalf("首轮应为月白清风，实际 %+v", rounds[0])
	}
	newest := rounds[len(rounds)-1]
	if newest.UserName != "钛宇星光阁" || newest.UserText != "第6句话" || newest.ReplyText != "回应6" {
		t.Fatalf("最新一轮应为第6句话，实际 %+v", newest)
	}
}
