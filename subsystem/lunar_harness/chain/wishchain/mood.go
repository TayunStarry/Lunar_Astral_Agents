package wishchain

// mood.go 心情生命周期：锚点初始化、快照、按最近 5 轮对话更新、愿望联动偏移。

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Init 初始化心情与态度锚点；失败不阻塞愿望读写，下一次更新会重试。
func Init(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	mu.Lock()
	ensureLoadedLocked()
	ready := initialized
	mu.Unlock()
	if ready {
		logger.Info(Module, "[心愿] 已恢复，当前心情=%s", SnapshotMood().Label)
		return nil
	}
	return initializeAnchors(ctx)
}

// SnapshotMood 返回只读心情快照；未初始化时返回兜底「开心」。
func SnapshotMood() MoodSnapshot {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	label := state.Mood.Label
	if label == "" {
		label = defaultLabel
	}
	weights := make(map[string]float64, len(state.Mood.Weights))
	maps.Copy(weights, state.Mood.Weights)
	return MoodSnapshot{Label: label, Weights: weights, Vector: append([]float32(nil), state.Mood.Vector...)}
}

// UpdateMoodFromText 基于单段文本更新心情（兼容入口）。
func UpdateMoodFromText(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return updateMoodText(ctx, text)
}

// UpdateMoodFromRounds 用最近 5 轮「用户发言 + 月华回应」更新心情。
//
// 窗口最新一轮没有前移时直接跳过，避免每轮生活 Tick 重复嵌入同一段内容。
func UpdateMoodFromRounds(ctx context.Context, rounds []Round) error {
	segment := lastMoodRounds(rounds, moodRoundWindow)
	if len(segment) == 0 {
		return nil
	}
	key := strings.TrimSpace(segment[len(segment)-1].Key)
	if key == "" {
		key = segment[len(segment)-1].UserText + "\x00" + segment[len(segment)-1].ReplyText
	}
	mu.Lock()
	if key == lastMoodWindowKey {
		mu.Unlock()
		return nil
	}
	mu.Unlock()

	if err := updateMoodText(ctx, joinRounds(segment)); err != nil {
		return err
	}
	mu.Lock()
	lastMoodWindowKey = key
	mu.Unlock()
	return nil
}

// NudgeMood 把当前心情朝指定锚点偏移；返回是否实际施加。
//
// 愿望联动（延期 / 临近 / 完成 / 逾期）统一走这里，保持与对话更新的同一套
// 向量空间与权重体系，避免出现两套互不相干的心情数值。
func NudgeMood(label string, strength float64) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	if strength <= 0 {
		return false
	}
	if strength > 1 {
		strength = 1
	}
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	target := state.Mood.Base[label]
	if len(target) == 0 {
		return false
	}
	current := state.Mood.Vector
	next := normalize(lerp(current, target, 1-strength))
	weights := make(map[string]float64, len(labels))
	for _, name := range labels {
		weights[name] = state.Mood.Weights[name] * (1 - strength)
	}
	weights[label] += strength
	// 归一化，保证权重总和恒为 1。
	var sum float64
	for _, weight := range weights {
		sum += weight
	}
	if sum > 0 {
		for name := range weights {
			weights[name] /= sum
		}
	}
	state.Mood.Vector = next
	state.Mood.Weights = weights
	state.Mood.Label = dominantLabel(weights, labels, label)
	state.Mood.UpdatedAt = time.Now().Unix()
	saveLocked()
	logger.Info(Module, "[心愿] 心情向「%s」偏移 %.2f，当前=%s", label, strength, state.Mood.Label)
	return true
}

// updateMoodText 用文本嵌入计算目标情绪并融入当前心情。
func updateMoodText(ctx context.Context, text string) error {
	if err := initializeAnchors(ctx); err != nil {
		return err
	}
	mu.Lock()
	base := copyBase(state.Mood.Base)
	current := append([]float32(nil), state.Mood.Vector...)
	mu.Unlock()
	if len(base) == 0 {
		return fmt.Errorf("基础情绪向量为空")
	}

	vecs, err := embedDocumentsHook(ctx, []string{text})
	if err != nil {
		return fmt.Errorf("计算发言嵌入失败: %w", err)
	}
	if len(vecs) == 0 {
		return fmt.Errorf("发言嵌入为空")
	}
	utterance := vecs[0]
	sims := make([]float64, len(labels))
	for i, label := range labels {
		sims[i] = cosine(utterance, base[label])
	}
	probs := softmax(sims, currentMoodTemperature, moodTemperature)
	target := weightedVector(base, probs, labels)
	if len(target) == 0 {
		return fmt.Errorf("基础情绪向量为空")
	}
	next := normalize(lerp(current, target, currentMoodAlpha))
	weights := make(map[string]float64, len(labels))
	for i, label := range labels {
		weights[label] = probs[i]
	}
	label := dominantLabel(weights, labels, defaultLabel)

	mu.Lock()
	state.Mood.Vector = next
	state.Mood.Weights = weights
	state.Mood.Label = label
	state.Mood.UpdatedAt = time.Now().Unix()
	saveLocked()
	mu.Unlock()
	logger.Info(Module, "[心愿] 心情更新完成，标签=%s", label)
	return nil
}

// initializeAnchors 计算心情基础情绪与态度两极锚点的嵌入向量。
func initializeAnchors(ctx context.Context) error {
	initMu.Lock()
	defer initMu.Unlock()

	mu.Lock()
	ensureLoadedLocked()
	if initialized {
		mu.Unlock()
		return nil
	}
	mu.Unlock()

	initCtx := ctx
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		initCtx, cancel = context.WithTimeout(ctx, initTimeoutSeconds*time.Second)
		defer cancel()
	}
	names := attitudeAnchorNames()
	texts := make([]string, 0, len(labels)+len(names))
	texts = append(texts, labels...)
	texts = append(texts, attitudeAnchorTexts(names)...)
	vecs, err := embedDocumentsHook(initCtx, texts)
	if err != nil {
		markFallbackMood()
		return fmt.Errorf("计算心情 / 态度锚点嵌入失败: %w", err)
	}
	if len(vecs) != len(texts) {
		markFallbackMood()
		return fmt.Errorf("锚点嵌入数量不匹配: 期望 %d, 实际 %d", len(texts), len(vecs))
	}
	dim := len(vecs[0])
	if dim == 0 {
		markFallbackMood()
		return fmt.Errorf("锚点嵌入为空")
	}

	base := make(map[string][]float32, len(labels))
	for i, label := range labels {
		if len(vecs[i]) != dim {
			markFallbackMood()
			return fmt.Errorf("基础情绪「%s」向量维度不一致", label)
		}
		base[label] = normalize(vecs[i])
	}
	anchors := make(map[string][]float32, len(names))
	for i, name := range names {
		vec := vecs[len(labels)+i]
		if len(vec) != dim {
			markFallbackMood()
			return fmt.Errorf("态度锚点「%s」向量维度不一致", name)
		}
		anchors[name] = normalize(vec)
	}

	mu.Lock()
	defer mu.Unlock()
	weights := make(map[string]float64, len(labels))
	if len(state.Mood.Weights) == len(labels) && len(state.Mood.Vector) == dim {
		maps.Copy(weights, state.Mood.Weights)
	} else {
		// 初始权重偏向开心：开心 70%，其余锚点均分剩下的 30%。
		for i, label := range labels {
			if i == 0 {
				weights[label] = 0.70
				continue
			}
			weights[label] = 0.30 / float64(len(labels)-1)
		}
	}
	weightList := make([]float64, len(labels))
	for i, label := range labels {
		weightList[i] = weights[label]
	}
	vector := normalize(weightedVector(base, weightList, labels))
	state.Model = config.AgentEmbeddingModel
	state.Dim = dim
	state.Mood.Base = base
	state.Mood.Vector = vector
	state.Mood.Weights = weights
	state.Mood.Label = dominantLabel(weights, labels, defaultLabel)
	state.Mood.UpdatedAt = time.Now().Unix()
	state.Attitude.Anchors = anchors
	state.Attitude.UpdatedAt = time.Now().Unix()
	initialized = true
	saveLocked()
	logger.Info(Module, "[心愿] 锚点初始化完成，维度=%d，心情=%s", dim, state.Mood.Label)
	return nil
}

// markFallbackMood 嵌入不可用时至少给出兜底标签，愿望仍可正常读写。
func markFallbackMood() {
	mu.Lock()
	defer mu.Unlock()
	if state.Mood.Label == "" {
		state.Mood.Label = defaultLabel
	}
}

// lastMoodRounds 取最近 window 轮有效交互。
func lastMoodRounds(rounds []Round, window int) []Round {
	valid := make([]Round, 0, len(rounds))
	for _, round := range rounds {
		userText := strings.TrimSpace(round.UserText)
		replyText := strings.TrimSpace(round.ReplyText)
		if userText == "" && replyText == "" {
			continue
		}
		valid = append(valid, Round{
			Key:       round.Key,
			UserName:  round.UserName,
			UserText:  userText,
			ReplyText: replyText,
		})
	}
	if len(valid) > window {
		valid = valid[len(valid)-window:]
	}
	return valid
}

// joinRounds 把窗口内的成对交互拼成一段用于嵌入的文本。
func joinRounds(rounds []Round) string {
	var builder strings.Builder
	for _, round := range rounds {
		if round.UserText != "" {
			builder.WriteString(round.UserText)
			builder.WriteString("\n")
		}
		if round.ReplyText != "" {
			builder.WriteString(round.ReplyText)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}
