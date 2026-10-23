package lunarloop

// action_match.go 动作词表的进程内匹配
//
// 动作词表来自渲染引擎的 animation_list 消息（见 anim_cache.go），是“当前这个角色
// 能做什么”的临时能力清单，不是月华的长期记忆。历史上曾把词表逐条写进记忆库再
// 做语义检索，带来三个问题：每轮重写、和真实记忆争 topK、清空记忆会弄坏动作。
//
// 现在词表只留在 AnimationListCache 里：本文件为动作名建立进程内向量索引，
// 查询时用同一套嵌入模型算余弦；嵌入不可用时退回包含匹配。全程不写记忆库。

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"

	"github.com/dop251/goja"
)

// actionMatchThreshold 动作名与动作文本的最低余弦相似度（低于此值不执行）
const actionMatchThreshold = 0.30

// actionVectorIndex 动作词表的进程内向量索引（词表签名变化时整体重建）
type actionVectorIndex struct {
	mu        sync.Mutex
	signature string
	vectors   [][]float32
}

var actionVectors actionVectorIndex

// actionMatchWarnAt 嵌入失败告警的限流时间（Unix 秒），避免每个动作块刷一条
var actionMatchWarnAt int64

// matchAction 绑定：matchAction(query) → 动作名
//
// 入参为动作/情绪文本（如“轻轻晃了晃脑袋”），返回 AnimationListCache 中最匹配的
// 动作名；没有可匹配动作时返回空串。匹配在 Go 侧完成，不读写记忆库。
func (class *Runtime) matchAction(call goja.FunctionCall) goja.Value {
	query := strings.TrimSpace(call.Argument(0).String())
	if query == "" {
		return class.runtime.ToValue("")
	}
	return class.runtime.ToValue(matchActionName(query))
}

// matchActionName 在动画列表缓存中为 query 选最佳动作名（语义优先，包含匹配兜底）
func matchActionName(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	cache := GetAnimationCache()
	cache.RLock()
	actions := append([]ActionDefinition(nil), cache.Actions...)
	cache.RUnlock()

	names := make([]string, 0, len(actions))
	for _, action := range actions {
		if name := strings.TrimSpace(action.Name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		// 引擎尚未上报动作列表：不猜动作名，避免把不存在的动作发给渲染引擎
		return ""
	}

	vectors, err := ensureActionVectors(names)
	if err != nil {
		warnActionEmbedding(err)
		return lexicalActionMatch(query, names)
	}

	// 动作匹配发生在对话回包路径上：嵌入查询设短超时，超时就退回字面匹配，
	// 不让动作匹配把整个对话链拖住。
	queryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queryVec, err := persistence.EmbedQueryText(queryCtx, query)
	if err != nil {
		warnActionEmbedding(err)
		return lexicalActionMatch(query, names)
	}

	bestName := ""
	bestSim := float32(-2)
	for i, name := range names {
		if len(vectors[i]) != len(queryVec) {
			continue
		}
		sim := actionCosine(queryVec, vectors[i])
		if sim > bestSim {
			bestSim = sim
			bestName = name
		}
	}
	if bestName != "" && bestSim >= actionMatchThreshold {
		logger.SubInfo("LunarCore", "Action", "动作匹配: %q → %q（相似度 %.3f）",
			truncateRunes(query, 40), bestName, bestSim)
		return bestName
	}

	// 语义不够接近时再看字面包含（例如动作名“挥手”直接出现在动作文本里）
	if matched := lexicalActionMatch(query, names); matched != "" {
		logger.SubInfo("LunarCore", "Action", "动作匹配（字面）: %q → %q", truncateRunes(query, 40), matched)
		return matched
	}
	return ""
}

// ensureActionVectors 返回与 names 一一对应的动作名向量；词表变化时重建索引。
func ensureActionVectors(names []string) ([][]float32, error) {
	signature := strings.Join(names, "\x00")

	actionVectors.mu.Lock()
	defer actionVectors.mu.Unlock()

	if actionVectors.signature == signature && len(actionVectors.vectors) == len(names) {
		return actionVectors.vectors, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vectors, err := persistence.EmbedDocumentTexts(ctx, names)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(names) {
		return nil, fmt.Errorf("动作名向量数量不匹配: 期望 %d, 实际 %d", len(names), len(vectors))
	}
	actionVectors.signature = signature
	actionVectors.vectors = vectors
	return vectors, nil
}

// lexicalActionMatch 字面兜底：精确相等或互相包含，取最长的动作名。
//
// 只接受明确的包含关系；不做模糊相似度，避免嵌入服务不可用时随机触发动作。
func lexicalActionMatch(query string, names []string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	best := ""
	for _, name := range names {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		if n == q {
			return name
		}
		if strings.Contains(q, n) || strings.Contains(n, q) {
			if len([]rune(name)) > len([]rune(best)) {
				best = name
			}
		}
	}
	return best
}

// actionCosine 计算两个向量的余弦相似度（维度不一致返回 -2）
func actionCosine(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return -2
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// warnActionEmbedding 嵌入失败告警限流（60 秒内最多一条）
func warnActionEmbedding(err error) {
	now := time.Now().Unix()
	last := atomic.LoadInt64(&actionMatchWarnAt)
	if now-last < 60 {
		return
	}
	if !atomic.CompareAndSwapInt64(&actionMatchWarnAt, last, now) {
		return
	}
	logger.Warn("LunarCore", "动作匹配嵌入失败（已退回字面匹配）: %v", err)
}

// truncateRunes 按字符截断（日志用，避免长动作文本刷屏）
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
