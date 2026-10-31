package mood

// mood.go 心情生命周期：初始化、快照、按最新发言更新。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const moodModule = "Mood"

// Init 启动阶段初始化心情；失败不阻塞调用方，保留可用兜底快照。
func Init(ctx context.Context) error {
	if loadState() {
		logger.Info(moodModule, "[心情] 已恢复，当前标签=%s", Snapshot().Label)
		return nil
	}
	if err := initialize(ctx); err != nil {
		logger.Warn(moodModule, "[心情] 初始化失败，暂用兜底标签「%s」: %v", defaultLabel, err)
		return err
	}
	logger.Info(moodModule, "[心情] 初始化完成，当前标签=%s，维度=%d", Snapshot().Label, state.Dim)
	return nil
}

// Snapshot 返回只读快照；未初始化时返回兜底「开心」。
func Snapshot() MoodSnapshot {
	mu.RLock()
	defer mu.RUnlock()
	label := state.Label
	if label == "" {
		label = defaultLabel
	}
	weights := make(map[string]float64, len(state.Weights))
	for key, value := range state.Weights {
		weights[key] = value
	}
	return MoodSnapshot{Label: label, Weights: weights, Vector: append([]float32(nil), state.Vector...)}
}

// UpdateFromText 基于最新发言更新心情；只允许生活体系调用。
func UpdateFromText(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if err := ensureInitialized(ctx); err != nil {
		return err
	}
	mu.RLock()
	base := copyBase(state.Base)
	current := append([]float32(nil), state.Vector...)
	mu.RUnlock()

	vecs, err := embedDocumentsHook(ctx, []string{text})
	if err != nil {
		return fmt.Errorf("计算发言嵌入失败: %w", err)
	}
	utterance := vecs[0]
	sims := make([]float64, len(labels))
	for i, label := range labels {
		sims[i] = cosine(utterance, base[label])
	}
	probs := softmax(sims, currentTemperature)
	target := weightedVector(base, probs)
	if len(target) == 0 {
		return fmt.Errorf("基础情绪向量为空")
	}
	next := normalize(lerp(current, target, currentAlpha))
	weights := make(map[string]float64, len(labels))
	for i, label := range labels {
		weights[label] = probs[i]
	}
	label := dominantLabel(weights)

	mu.Lock()
	state.Vector = next
	state.Weights = weights
	state.Label = label
	saveStateLocked()
	mu.Unlock()
	logger.Info(moodModule, "[心情] 更新完成，标签=%s", label)
	return nil
}

// ensureInitialized 按需初始化；并发调用由 initialize 的二次检查兜住。
func ensureInitialized(ctx context.Context) error {
	mu.RLock()
	ready := initialized
	mu.RUnlock()
	if ready {
		return nil
	}
	return initialize(ctx)
}

func initialize(ctx context.Context) error {
	initMu.Lock()
	defer initMu.Unlock()
	mu.RLock()
	if initialized {
		mu.RUnlock()
		return nil
	}
	mu.RUnlock()

	initCtx := ctx
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		initCtx, cancel = context.WithTimeout(ctx, initTimeoutSeconds*time.Second)
		defer cancel()
	}
	vecs, err := embedDocumentsHook(initCtx, labels)
	if err != nil {
		return fmt.Errorf("计算基础情绪嵌入失败: %w", err)
	}
	if len(vecs) != len(labels) {
		return fmt.Errorf("基础情绪嵌入数量不匹配: 期望 %d, 实际 %d", len(labels), len(vecs))
	}
	dim := len(vecs[0])
	if dim == 0 {
		return fmt.Errorf("基础情绪嵌入为空")
	}
	base := make(map[string][]float32, len(labels))
	for i, label := range labels {
		if len(vecs[i]) != dim {
			return fmt.Errorf("基础情绪「%s」向量维度不一致", label)
		}
		base[label] = normalize(vecs[i])
	}
	// 初始权重偏向开心：开心 70%，其余基础情绪均分剩下的 30%。
	initWeights := make([]float64, len(labels))
	for i := range initWeights {
		if i == 0 {
			initWeights[i] = 0.70
			continue
		}
		initWeights[i] = 0.30 / float64(len(labels)-1)
	}
	vector := normalize(weightedVector(base, initWeights))
	weights := make(map[string]float64, len(labels))
	for i, label := range labels {
		weights[label] = initWeights[i]
	}
	next := State{
		Version: 1,
		Model:   config.AgentEmbeddingModel,
		Dim:     dim,
		Base:    base,
		Vector:  vector,
		Weights: weights,
		Label:   dominantLabel(weights),
	}
	mu.Lock()
	state = next
	initialized = true
	saveStateLocked()
	mu.Unlock()
	return nil
}

func copyBase(src map[string][]float32) map[string][]float32 {
	out := make(map[string][]float32, len(src))
	for key, value := range src {
		out[key] = append([]float32(nil), value...)
	}
	return out
}
