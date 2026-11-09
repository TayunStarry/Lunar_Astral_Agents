package wishchain

// attitude.go 态度（对每个用户的好感认知）：用最近 5 轮成对语义在双极轴上累积。
//
// 态度与心情是两套独立机制：心情看全局对话氛围，态度只看「月华对这个用户」
// 的成对交互，但两者共享同一个嵌入模型与同一份持久化文件。

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// UpdateAttitudeWindows 用每个用户最近若干轮的成对语义窗口更新态度认知。
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
	pending := make(map[string][]Round, len(windows))
	for userName, window := range windows {
		newest := attitudeRoundKey(window[len(window)-1])
		if previous, ok := state.Attitude.Users[userName]; ok && previous.LastKey == newest {
			continue
		}
		pending[userName] = window
	}
	mu.Unlock()
	if len(pending) == 0 {
		return 0, nil
	}

	names := make([]string, 0, len(pending))
	texts := make([]string, 0, len(pending)*attitudeRoundWindow)
	for userName := range pending {
		names = append(names, userName)
	}
	sort.Strings(names)
	for _, userName := range names {
		for _, round := range pending[userName] {
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
		window := pending[userName]
		var sample float64
		for i := 0; i < len(window); i++ {
			sample += attitudePosture(vecs[offset], anchors)
			offset++
		}
		sample /= float64(len(window))
		previous := state.Attitude.Users[userName]
		next := sample
		if previous.Samples > 0 {
			next = currentAttitudeAlpha*previous.Score + (1-currentAttitudeAlpha)*sample
		}
		label := attitudeLabelFor(next)
		state.Attitude.Users[userName] = UserAffection{
			Score:        next,
			Label:        label,
			Samples:      previous.Samples + 1,
			WindowRounds: len(window),
			LastKey:      attitudeRoundKey(window[len(window)-1]),
			UpdatedAt:    time.Now().Unix(),
		}
		logger.Info(Module, "[心愿] 对用户「%s」好感度 %.2f（%s），评估窗口 %d 轮", userName, next, label, len(window))
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

// AttitudeScore 返回月华对指定用户的好感度原始值（[-1, 1]）。
func AttitudeScore(userName string) (float64, bool) {
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
	return affection.Score, true
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

// attitudeLabelFor 按好感度阈值给出情感标签。
func attitudeLabelFor(score float64) string {
	switch {
	case score >= strongPositiveThreshold:
		return "很喜欢"
	case score >= positiveThreshold:
		return "喜欢"
	case score <= strongNegativeThreshold:
		return "讨厌"
	case score <= negativeThreshold:
		return "疏离"
	default:
		return defaultAttitudeLabel
	}
}
