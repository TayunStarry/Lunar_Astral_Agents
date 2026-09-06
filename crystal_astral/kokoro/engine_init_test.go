package kokoro

import (
	"path/filepath"
	"testing"
)

// TestEngineInitDebug 复现引擎初始化以抓取 panic（临时排查用）
func TestEngineInitDebug(t *testing.T) {
	modelDir := "d:/Lunar_Astral_Agents/local_data/models/Kokoro-82M-v1.1-zh"
	SetOnnxLibraryPath(filepath.Join(modelDir, "onnxruntime.dll"))
	if err := InitEngine(modelDir, filepath.Join(modelDir, "voices"), filepath.Join(modelDir, "espeak")); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	t.Log("init ok")
}
