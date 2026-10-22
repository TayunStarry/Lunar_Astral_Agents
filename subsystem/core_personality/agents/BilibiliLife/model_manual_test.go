package BilibiliLife

// 手动冒烟：文本模型流式输出（会写进核心智能实况）。
//   $env:BILI_LIFE_MODEL_PROBE="1"; go test -run TestManualModelProbe -v ./agents/BilibiliLife

import (
	"os"
	"testing"
)

func TestManualModelProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_MODEL_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_MODEL_PROBE=1 才调用模型")
	}
	text, err := chatText("只回复两个字：收到。", "现在测试流式输出。")
	if err != nil {
		t.Fatalf("模型调用失败: %v", err)
	}
	t.Logf("模型输出: %s", truncate(text, 120))
}
