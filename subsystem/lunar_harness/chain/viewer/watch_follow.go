package viewer

// watch_follow.go 评论 @ 的目标来源与一次发布会话。
//
// B 站关注分组接口 x/relation/tags 里「特别关注」是固定 tagid=-10（默认分组为 0），
// 这里直接按该分组读取成员，不在本地配置里另存一份名单；读取失败或为空时降级为不 @。
//
// 候选人只以「展示名 + 序号」交给模型，内部 UID 不进入提示词；模型指定的人必须能在
// 本次候选清单里按名字或序号对上，认不出的直接丢弃，绝不拿它去搜人或凭编造的用户名发 @。

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/bilibili"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// closingMentionMax 一次评论最多 @ 的特别关注人数。
	closingMentionMax = 3
)

// specialFollows 读取当前账号「特别关注」分组里的用户。
func (w *webSession) specialFollows() ([]MentionTarget, error) {
	return bilibili.SpecialFollows(w)
}

// browserMentionSession 复用同一个浏览器会话完成「读候选 → 发评论」。
type browserMentionSession struct {
	w          *webSession
	candidates []MentionTarget
}

// openMentionSession 打开浏览器并读取特别关注候选；读取候选失败不阻断评论发布。
func openMentionSession() (mentionSession, error) {
	w, err := openBilibiliBrowser()
	if err != nil {
		return nil, err
	}
	pool, followErr := w.specialFollows()
	if followErr != nil {
		logger.Warn("BilibiliViewer", "[看视频] 读取特别关注失败，评论降级为不 @: %v", followErr)
	}
	return &browserMentionSession{w: w, candidates: pool}, nil
}

func (s *browserMentionSession) Candidates() []MentionTarget { return s.candidates }

func (s *browserMentionSession) Close() {
	if s != nil && s.w != nil {
		s.w.close()
	}
}

// Post 逐条发布评论；每条只挂自己的 @ 目标，互不影响。
func (s *browserMentionSession) Post(video viewerstore.WatchVideo, comments []mentionComment, aid int64) (int, bool, error) {
	if err := s.w.ensureLogin(); err != nil {
		return 0, false, err
	}
	if aid <= 0 {
		info, infoErr := bilibili.FetchVideo(video.BV)
		if infoErr != nil {
			return 0, false, infoErr
		}
		aid = info.Aid
	}
	if aid <= 0 {
		return 0, false, fmt.Errorf("视频 aid 缺失，无法发评论")
	}
	oid := strconv.FormatInt(aid, 10)
	posted, mentionPosted := 0, false
	var firstErr error
	for _, comment := range comments {
		at := comment.Targets
		if len(at) > closingMentionMax {
			at = at[:closingMentionMax]
		}
		if err := s.w.postCommentOnObject(1, oid, comment.Text, at); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		posted++
		if len(at) > 0 {
			mentionPosted = true
		}
	}
	return posted, mentionPosted, firstErr
}

// resolveMentions 把一条评论的 @ 意图收束成最终名单。
//
// 人数为 0、候选为空时一律不 @。人数 > 0 时优先使用模型指定的候选：
// 按候选清单校验（名字或 1 基序号），认不出的直接丢弃；一个都没认出来才回退随机挑选。
func resolveMentions(pool []MentionTarget, mention int, at []string, randInt func(int) int) []MentionTarget {
	if len(pool) == 0 || mention <= 0 {
		return nil
	}
	if named := matchMentions(pool, at); len(named) > 0 {
		return named
	}
	return pickMentions(pool, mention, randInt)
}

// matchMentions 按名字或候选序号（1 基）在清单里找目标；不认识的直接丢弃。
func matchMentions(pool []MentionTarget, wants []string) []MentionTarget {
	out := make([]MentionTarget, 0, closingMentionMax)
	seen := make(map[int64]bool, closingMentionMax)
	for _, raw := range wants {
		want := strings.TrimSpace(raw)
		if want == "" {
			continue
		}
		var found MentionTarget
		ok := false
		if index, err := strconv.Atoi(want); err == nil {
			if index >= 1 && index <= len(pool) {
				found, ok = pool[index-1], true
			}
		} else {
			for _, target := range pool {
				if target.Name == want {
					found, ok = target, true
					break
				}
			}
		}
		if !ok || seen[found.UID] {
			continue
		}
		seen[found.UID] = true
		out = append(out, found)
		if len(out) >= closingMentionMax {
			break
		}
	}
	return out
}

// pickMentions 从特别关注里随机挑 want 人（want 钳制到 1-3）；候选不足时取全部。
// randInt 由调用方注入，测试可固定取值。
func pickMentions(pool []MentionTarget, want int, randInt func(int) int) []MentionTarget {
	if len(pool) == 0 || want <= 0 {
		return nil
	}
	if want > closingMentionMax {
		want = closingMentionMax
	}
	if want > len(pool) {
		want = len(pool)
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

// defaultMentionRand 生产环境的随机源。
func defaultMentionRand(n int) int { return rand.Intn(n) }
