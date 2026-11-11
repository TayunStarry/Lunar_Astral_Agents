package viewer

// watch_follow.go 收束评论的 @ 目标来源。
//
// B 站关注分组接口 x/relation/tags 里「特别关注」是固定 tagid=-10（默认分组为 0），
// 这里直接按该分组读取成员，不在本地配置里另存一份名单；读取失败或为空时降级为不 @。

import (
	"math/rand"

	"LunarSubsystem/LunarHarness/engine/bilibili"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// closingMentionMin / closingMentionMax 收束评论随机 @ 的人数范围。
	closingMentionMin = 1
	closingMentionMax = 3
)

// specialFollows 读取当前账号「特别关注」分组里的用户。
func (w *webSession) specialFollows() ([]MentionTarget, error) {
	return bilibili.SpecialFollows(w)
}

// closingMentionTargets 读取特别关注并随机挑 1-3 人；失败或为空时降级为不 @，并记录原因。
func closingMentionTargets(session *webSession) []MentionTarget {
	pool, err := session.specialFollows()
	if err != nil {
		logger.Warn("BilibiliViewer", "[看视频] 读取特别关注失败，收束评论降级为不 @: %v", err)
		return nil
	}
	if len(pool) == 0 {
		logger.Info("BilibiliViewer", "[看视频] 特别关注列表为空，收束评论不 @")
		return nil
	}
	targets := pickClosingMentions(pool, rand.Intn)
	logger.Info("BilibiliViewer", "[看视频] 收束评论将 @ %d 位特别关注", len(targets))
	return targets
}

// pickClosingMentions 随机挑 1-3 个特别关注；候选不足时取全部，空列表返回 nil。
// randInt 由调用方注入，测试可固定取值。
func pickClosingMentions(pool []MentionTarget, randInt func(int) int) []MentionTarget {
	if len(pool) == 0 {
		return nil
	}
	want := closingMentionMin + randInt(closingMentionMax-closingMentionMin+1)
	if want > len(pool) {
		want = len(pool)
	}
	if want < closingMentionMin {
		want = closingMentionMin
	}
	shuffled := append([]MentionTarget(nil), pool...)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := randInt(i + 1)
		if j < 0 || j > i {
			j = i
		}
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	return shuffled[:want]
}
