package mood

import "math"

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

// softmax 把相似度转成情绪分布。
func softmax(values []float64, temp float64) []float64 {
	if temp <= 0 {
		temp = currentTemperature
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

// lerp 向量线性插值：weightA 是 a 的权重。
func lerp(a, b []float32, weightA float64) []float32 {
	if len(a) == 0 {
		return append([]float32(nil), b...)
	}
	if len(b) == 0 || len(a) != len(b) {
		return append([]float32(nil), a...)
	}
	out := make([]float32, len(a))
	for i := range a {
		out[i] = float32(weightA*float64(a[i]) + (1-weightA)*float64(b[i]))
	}
	return out
}

// weightedVector 按情绪权重合成目标向量。
func weightedVector(base map[string][]float32, weights []float64) []float32 {
	var out []float32
	for i, label := range labels {
		vec := base[label]
		if len(vec) == 0 {
			continue
		}
		if out == nil {
			out = make([]float32, len(vec))
		}
		for d := range out {
			if d < len(vec) {
				out[d] += float32(weights[i]) * vec[d]
			}
		}
	}
	return out
}

// bestLabel 返回与当前向量最相似的基础情绪标签。
func bestLabel(vec []float32, base map[string][]float32, weights map[string]float64) string {
	best := defaultLabel
	bestScore := -2.0
	for _, label := range labels {
		score := cosine(vec, base[label])
		if score > bestScore || (score == bestScore && weights[label] > weights[best]) {
			best = label
			bestScore = score
		}
	}
	return best
}
