package affection

// state.go 情感认知状态的读写与基础向量运算。

import (
	"encoding/json"
	"math"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// loadState 读取持久化情感认知；不满足当前嵌入模型或锚点缺失时返回 false。
func loadState() bool {
	raw, ok := persistence.ReadFileBytes(stateFile)
	if !ok {
		return false
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(Module, "[情感] 状态文件损坏，将重新初始化: %v", err)
		return false
	}
	if saved.Model != config.AgentEmbeddingModel || saved.Dim <= 0 {
		logger.Info(Module, "[情感] 嵌入模型或向量维度已变化，重新初始化")
		return false
	}
	for _, name := range anchorNames() {
		if len(saved.Anchors[name]) != saved.Dim {
			logger.Info(Module, "[情感] 情感锚点缺失或维度不一致，重新初始化")
			return false
		}
	}
	if saved.Users == nil {
		saved.Users = make(map[string]UserAffection)
	}
	mu.Lock()
	state = saved
	initialized = true
	mu.Unlock()
	return true
}

// saveStateLocked 持久化当前状态；调用方需持有写锁。
func saveStateLocked() {
	state.Version = 1
	state.UpdatedAt = time.Now().Unix()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn(Module, "[情感] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(stateFile, raw); err != nil {
		logger.Warn(Module, "[情感] 状态写入失败: %v", err)
	}
}

// cosine 余弦相似度；零向量返回 0。
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// normalize 归一化向量；零向量原样返回。
func normalize(vec []float32) []float32 {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return vec
	}
	inv := float32(1 / math.Sqrt(norm))
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = v * inv
	}
	return out
}

// softmax 把相似度转成极性分布。
func softmax(values []float64, temp float64) []float64 {
	if temp <= 0 {
		temp = affectionTemperature
	}
	maxValue := values[0]
	for _, v := range values[1:] {
		if v > maxValue {
			maxValue = v
		}
	}
	out := make([]float64, len(values))
	var sum float64
	for i, v := range values {
		out[i] = math.Exp((v - maxValue) / temp)
		sum += out[i]
	}
	if sum == 0 {
		for i := range out {
			out[i] = 1 / float64(len(out))
		}
		return out
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}
