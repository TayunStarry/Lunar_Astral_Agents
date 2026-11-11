package wishchain

// attitude.go 态度（对每个用户的情感认知）：按最近对话的感情极性做积分增减。
//
// 态度与心情是两套独立机制：心情看全局对话氛围，态度只看「月华对这个用户」
// 的成对交互，但两者共享同一个嵌入模型与同一份持久化文件。
//
// 积分规则：每个用户从 20 分起步，范围 [-50, 50]；每次只评估窗口里尚未
// 评估过的新增轮次，按感情极性程度加分或扣分，不再做历史平均，避免态度
// 长期被拉回中间值。

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// UpdateAttitudeWindows 用每个用户最近若干轮成对语义中的新增轮次更新态度积分。
func UpdateAttitudeWindows(ctx context.Context, rounds []Round) (int, error) {
	windows := groupAttitudeWindows(rounds)
	if len(windows) == 0 {
		return 0, nil
	}
	if err := initializeAnchors(ctx); err != nil {
		return 0, err
	}

	mu.Lock()
	ensureLoadedLocked()
	anchors := copyAnchors(state.Attitude.Anchors)
	fresh := make(map[string][]Round, len(windows))
	for userName, window := range windows {
		previous := state.Attitude.Users[userName]
		if pending := roundsAfter(window, previous.LastKey); len(pending) > 0 {
			fresh[userName] = pending
		}
	}
	mu.Unlock()
	if len(fresh) == 0 {
		return 0, nil
	}

	names := make([]string, 0, len(fresh))
	for userName := range fresh {
		names = append(names, userName)
	}
	sort.Strings(names)
	texts := make([]string, 0, len(fresh))
	for _, userName := range names {
		for _, round := range fresh[userName] {
			texts = append(texts, round.UserText+"\n"+round.ReplyText)
		}
	}

	vecs, err := embedDocumentsHook(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("计算成对语义嵌入失败: %w", err)
	}
	if len(vecs) != len(texts) {
		return 0, fmt.Errorf("成对语义嵌入数量不匹配: 期望 %d, 实际 %d", len(texts), len(vecs))
	}

	mu.Lock()
	defer mu.Unlock()
	if state.Attitude.Users == nil {
		state.Attitude.Users = make(map[string]UserAffection)
	}
	offset := 0
	for _, userName := range names {
		pending := fresh[userName]
		var posture float64
		for range pending {
			posture += attitudePosture(vecs[offset], anchors)
			offset++
		}
		posture /= float64(len(pending))
		delta := int(math.Round(posture * attitudeMaxStep))

		previous := state.Attitude.Users[userName]
		points := previous.Points
		if previous.Samples == 0 {
			points = attitudeDefaultPoints
		}
		points = clampAttitudePoints(points + delta)
		label := attitudeLabelFor(points)
		state.Attitude.Users[userName] = UserAffection{
			Points:       points,
			Label:        label,
			Samples:      previous.Samples + 1,
			WindowRounds: len(pending),
			LastKey:      attitudeRoundKey(pending[len(pending)-1]),
			UpdatedAt:    time.Now().Unix(),
		}
		logger.Info(Module, "[心愿] 对用户「%s」态度分 %d（%+d，%s），本次评估 %d 轮",
			userName, points, delta, label, len(pending))
	}
	state.Attitude.UpdatedAt = time.Now().Unix()
	saveLocked()
	return len(names), nil
}

// AttitudeLabel 返回月华对指定用户已形成的情感标签。
func AttitudeLabel(userName string) (string, bool) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return "", false
	}
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	affection, ok := state.Attitude.Users[userName]
	if !ok || affection.Label == "" {
		return "", false
	}
	return affection.Label, true
}

// AttitudeScore 返回月华对指定用户的态度积分（[-50, 50]）。
func AttitudeScore(userName string) (int, bool) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return 0, false
	}
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	affection, ok := state.Attitude.Users[userName]
	if !ok || affection.Samples == 0 {
		return 0, false
	}
	return affection.Points, true
}

// groupAttitudeWindows 按用户分组、每人只保留最近 window 轮；成对要素缺失的轮次直接丢弃。
func groupAttitudeWindows(rounds []Round) map[string][]Round {
	windows := make(map[string][]Round)
	for _, round := range rounds {
		userName := strings.TrimSpace(round.UserName)
		userText := strings.TrimSpace(round.UserText)
		replyText := strings.TrimSpace(round.ReplyText)
		if userName == "" || userText == "" || replyText == "" {
			continue
		}
		window := append(windows[userName], Round{
			Key:       strings.TrimSpace(round.Key),
			UserName:  userName,
			UserText:  userText,
			ReplyText: replyText,
		})
		if len(window) > attitudeRoundWindow {
			window = window[len(window)-attitudeRoundWindow:]
		}
		windows[userName] = window
	}
	return windows
}

// roundsAfter 返回窗口里晚于 lastKey 的轮次；lastKey 已滑出窗口时返回整个窗口。
func roundsAfter(window []Round, lastKey string) []Round {
	lastKey = strings.TrimSpace(lastKey)
	if lastKey == "" {
		return window
	}
	for i := len(window) - 1; i >= 0; i-- {
		if attitudeRoundKey(window[i]) == lastKey {
			return window[i+1:]
		}
	}
	return window
}

// attitudeRoundKey 给一轮交互生成去重标识；缺少外部 ID 时退回发言与回应本身。
func attitudeRoundKey(round Round) string {
	if key := strings.TrimSpace(round.Key); key != "" {
		return key
	}
	return round.UserText + "\x00" + round.ReplyText
}

// attitudePosture 计算成对语义在双极好感轴上的极性：正向概率和减去负向概率和。
func attitudePosture(vec []float32, anchors map[string][]float32) float64 {
	sims := make([]float64, 0, len(positiveLabels)+len(negativeLabels))
	for _, name := range positiveLabels {
		sims = append(sims, cosine(vec, anchors[name]))
	}
	for _, name := range negativeLabels {
		sims = append(sims, cosine(vec, anchors[name]))
	}
	probs := softmax(sims, currentAttitudeTemperature, attitudeTemperature)
	var positive, negative float64
	for i := range positiveLabels {
		positive += probs[i]
	}
	for i := len(positiveLabels); i < len(probs); i++ {
		negative += probs[i]
	}
	return positive - negative
}

// attitudeLabelFor 按态度积分给出情感标签。
func attitudeLabelFor(points int) string {
	switch {
	case points >= attitudeStrongPositiveThreshold:
		return "很喜欢"
	case points >= attitudePositiveThreshold:
		return "喜欢"
	case points <= attitudeStrongNegativeThreshold:
		return "讨厌"
	case points <= attitudeNegativeThreshold:
		return "疏离"
	default:
		return defaultAttitudeLabel
	}
}

// clampAttitudePoints 把积分限制在 [-50, 50]。
func clampAttitudePoints(points int) int {
	if points < attitudeMinPoints {
		return attitudeMinPoints
	}
	if points > attitudeMaxPoints {
		return attitudeMaxPoints
	}
	return points
}

// legacyPoints 把旧版 [-1, 1] 好感度换算成态度积分。
func legacyPoints(score float64) int {
	if score > 1 {
		score = 1
	}
	if score < -1 {
		score = -1
	}
	return clampAttitudePoints(attitudeDefaultPoints + int(math.Round(score*30)))
}
