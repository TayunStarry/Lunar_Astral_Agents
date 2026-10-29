package mood

// type.go 心情子系统的类型定义。

// State 心情持久化状态。
//
// Base 保存各基础情绪的标准嵌入向量；Vector 是当前多维心情。
// 两者必须在同一嵌入模型、同一向量空间下，因此一起持久化 Model / Dim。
type State struct {
	Version   int                  `json:"version"`
	Model     string               `json:"model"`
	Dim       int                  `json:"dim"`
	Base      map[string][]float32 `json:"base_vectors"`
	Vector    []float32            `json:"vector"`
	Weights   map[string]float64   `json:"weights"`
	Label     string               `json:"label"`
	UpdatedAt int64                `json:"updated_at"`
}

// MoodSnapshot 供对话运行时消息读取的只读快照。
type MoodSnapshot struct {
	Label   string
	Weights map[string]float64
	Vector  []float32
}
