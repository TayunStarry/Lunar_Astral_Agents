package module

import (
	"encoding/json"
	"fmt"
	"os"
)

// tokenizerFile tokenizer.json 的解析结构
type tokenizerFile struct {
	Model struct {
		Vocab map[string]int64 `json:"vocab"`
	} `json:"model"`
}

// NewTokenizer 从 tokenizer.json 加载字符级音素词表
func NewTokenizer(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 tokenizer.json 失败: %w", err)
	}
	var tf tokenizerFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("解析 tokenizer.json 失败: %w", err)
	}
	vocab := make(map[rune]int64, len(tf.Model.Vocab))
	for k, v := range tf.Model.Vocab {
		runes := []rune(k)
		// 词表按单字符组织
		if len(runes) == 1 {
			vocab[runes[0]] = v
		}
	}
	if len(vocab) == 0 {
		return nil, fmt.Errorf("tokenizer 词表为空")
	}
	return &Tokenizer{vocab: vocab}, nil
}

// Tokenize 将音素字符串逐字符映射为 token id，不在词表中的字符被过滤
func (t *Tokenizer) Tokenize(phonemes string) []int64 {
	ids := make([]int64, 0, len([]rune(phonemes)))
	for _, r := range phonemes {
		if id, ok := t.vocab[r]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}
