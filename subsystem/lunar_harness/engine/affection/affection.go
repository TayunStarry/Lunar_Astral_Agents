package affection

// affection.go 情感认知：用「用户消息 + 月华回应」的成对语义，
// 在双极好感轴上累积出对每个用户的独立情感认知。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Init 启动阶段初始化两极锚点；失败不阻塞调用方，下一次更新会重试初始化。
func Init(ctx context.Context) error {
	if loadState() {
		logger.Info(Module, "[情感] 已恢复，已认知用户 %d 位", len(state.Users))
		return nil
	}
	if err := initialize(ctx); err != nil {
		logger.Warn(Module, "[情感] 初始化失败，暂不产生情感标签: %v", err)
		return err
	}
	logger.Info(Module, "[情感] 初始化完成，锚点=%d 个", len(state.Anchors))
	return nil
}

// Update 用一次成对交互（用户发言 + 月华回应）更新对指定用户的长期好感度。
// 三者任一为空时跳过：成对语义缺一半就不构成一次有效的情感交互。
func Update(ctx context.Context, userName, userText, replyText string) error {
	userName = strings.TrimSpace(userName)
	userText = strings.TrimSpace(userText)
	replyText = strings.TrimSpace(replyText)
	if userName == "" || userText == "" || replyText == "" {
		return nil
	}
	if err := ensureInitialized(ctx); err != nil {
		return err
	}

	mu.RLock()
	anchors := copyAnchors(state.Anchors)
	previous := state.Users[userName]
	mu.RUnlock()

	vecs, err := embedDocumentsHook(ctx, []string{userText + "\n" + replyText})
	if err != nil {
		return fmt.Errorf("计算成对语义嵌入失败: %w", err)
	}
	sample := posture(vecs[0], anchors)

	next := sample
	if previous.Samples > 0 {
		next = currentAlpha*previous.Score + (1-currentAlpha)*sample
	}
	label := labelFor(next)

	mu.Lock()
	if state.Users == nil {
		state.Users = make(map[string]UserAffection)
	}
	state.Users[userName] = UserAffection{
		Score:     next,
		Label:     label,
		Samples:   previous.Samples + 1,
		UpdatedAt: time.Now().Unix(),
	}
	saveStateLocked()
	mu.Unlock()
	logger.Info(Module, "[情感] 对用户「%s」好感度 %.2f（%s）", userName, next, label)
	return nil
}

// Label 返回月华对指定用户已形成的情感标签；未认知时返回 false。
func Label(userName string) (string, bool) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return "", false
	}
	mu.RLock()
	defer mu.RUnlock()
	affection, ok := state.Users[userName]
	if !ok || affection.Label == "" {
		return "", false
	}
	return affection.Label, true
}

// Score 返回月华对指定用户的好感度原始值（[-1, 1]）；未认知时返回 false。
func Score(userName string) (float64, bool) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return 0, false
	}
	mu.RLock()
	defer mu.RUnlock()
	affection, ok := state.Users[userName]
	if !ok || affection.Samples == 0 {
		return 0, false
	}
	return affection.Score, true
}

// posture 计算成对语义在双极好感轴上的极性：正向概率和减去负向概率和。
func posture(vec []float32, anchors map[string][]float32) float64 {
	sims := make([]float64, 0, len(positiveLabels)+len(negativeLabels))
	for _, name := range positiveLabels {
		sims = append(sims, cosine(vec, anchors[name]))
	}
	for _, name := range negativeLabels {
		sims = append(sims, cosine(vec, anchors[name]))
	}
	probs := softmax(sims, currentTemperature)
	var positive, negative float64
	for i := range positiveLabels {
		positive += probs[i]
	}
	for i := len(positiveLabels); i < len(probs); i++ {
		negative += probs[i]
	}
	return positive - negative
}

// labelFor 按好感度阈值给出情感标签。
func labelFor(score float64) string {
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
		return defaultLabel
	}
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
	names := anchorNames()
	vecs, err := embedDocumentsHook(initCtx, anchorTextsOf(names))
	if err != nil {
		return fmt.Errorf("计算情感锚点嵌入失败: %w", err)
	}
	if len(vecs) != len(names) {
		return fmt.Errorf("情感锚点嵌入数量不匹配: 期望 %d, 实际 %d", len(names), len(vecs))
	}
	dim := len(vecs[0])
	if dim == 0 {
		return fmt.Errorf("情感锚点嵌入为空")
	}
	anchors := make(map[string][]float32, len(names))
	for i, name := range names {
		if len(vecs[i]) != dim {
			return fmt.Errorf("情感锚点「%s」向量维度不一致", name)
		}
		anchors[name] = normalize(vecs[i])
	}
	mu.Lock()
	state = State{
		Version: 1,
		Model:   config.AgentEmbeddingModel,
		Dim:     dim,
		Anchors: anchors,
		Users:   make(map[string]UserAffection),
	}
	initialized = true
	saveStateLocked()
	mu.Unlock()
	return nil
}

func anchorNames() []string {
	names := make([]string, 0, len(positiveLabels)+len(negativeLabels))
	names = append(names, positiveLabels...)
	names = append(names, negativeLabels...)
	return names
}

func anchorTextsOf(names []string) []string {
	texts := make([]string, 0, len(names))
	for _, name := range names {
		texts = append(texts, anchorTexts[name])
	}
	return texts
}

func copyAnchors(src map[string][]float32) map[string][]float32 {
	out := make(map[string][]float32, len(src))
	for name, vec := range src {
		out[name] = append([]float32(nil), vec...)
	}
	return out
}
