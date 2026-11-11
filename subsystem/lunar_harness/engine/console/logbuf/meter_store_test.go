package logbuf

// meter_store_test.go 校验模型调用计量的持久化与损坏文件处理。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// useTempLocalDir 把 LocalDir 指到临时目录，并在用例结束后恢复。
func useTempLocalDir(t *testing.T) {
	t.Helper()
	old := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = old })
}

// resetMeter 清空计量与落盘状态；lastSave 设为当前时刻，避免用例中触发后台写盘。
func resetMeter(t *testing.T) {
	t.Helper()
	meterMu.Lock()
	saved := meterStats
	savedDirty := meterDirty
	savedSaving := meterSaving
	savedLast := meterLastSave
	meterStats = ModelCallStats{}
	meterDirty = false
	meterSaving = false
	meterLastSave = time.Now()
	meterMu.Unlock()
	t.Cleanup(func() {
		meterMu.Lock()
		meterStats = saved
		meterDirty = savedDirty
		meterSaving = savedSaving
		meterLastSave = savedLast
		meterMu.Unlock()
	})
}

func TestModelStatsPersistAcrossRestart(t *testing.T) {
	useTempLocalDir(t)
	resetMeter(t)

	RecordModelCall(1200, 300, 800, 2*time.Second, false)
	RecordModelCall(900, 150, 0, time.Second, true)
	if err := SaveModelStats(); err != nil {
		t.Fatalf("保存计量失败: %v", err)
	}

	// 模拟重启：清空内存后从磁盘读回。
	meterMu.Lock()
	meterStats = ModelCallStats{}
	meterDirty = false
	meterMu.Unlock()
	if err := LoadModelStats(); err != nil {
		t.Fatalf("读取计量失败: %v", err)
	}

	got := GetModelCallStats()
	if got.Calls != 2 || got.PromptTokens != 2100 || got.CompletionTokens != 450 {
		t.Fatalf("累计值不正确: %+v", got)
	}
	if got.CachedPromptTokens != 800 {
		t.Fatalf("估算调用不应累计缓存命中，实际 %d", got.CachedPromptTokens)
	}
	if got.MaxPromptTokens != 1200 || got.MaxCompletionTokens != 300 {
		t.Fatalf("峰值不正确: %+v", got)
	}
	if got.GenerationNanos != int64(3*time.Second) {
		t.Fatalf("生成耗时累计不正确: %d", got.GenerationNanos)
	}
	if got.EstimatedCalls != 1 {
		t.Fatalf("估算调用计数不正确: %d", got.EstimatedCalls)
	}
}

func TestLoadModelStatsMissingFileStartsFromZero(t *testing.T) {
	useTempLocalDir(t)
	resetMeter(t)

	if err := LoadModelStats(); err != nil {
		t.Fatalf("文件不存在不应报错: %v", err)
	}
	if got := GetModelCallStats(); got.Calls != 0 {
		t.Fatalf("首次启动应从零开始，实际 %+v", got)
	}
}

func TestLoadModelStatsRejectsBrokenFile(t *testing.T) {
	useTempLocalDir(t)
	resetMeter(t)

	path := meterFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte("{不是合法 JSON"), 0o644); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}
	if err := LoadModelStats(); err == nil {
		t.Fatal("损坏文件应返回错误")
	}
	if got := GetModelCallStats(); got.Calls != 0 {
		t.Fatalf("解析失败不应污染内存计量，实际 %+v", got)
	}
}

func TestSaveModelStatsSkipsWhenClean(t *testing.T) {
	useTempLocalDir(t)
	resetMeter(t)

	if err := SaveModelStats(); err != nil {
		t.Fatalf("无新数据时保存不应报错: %v", err)
	}
	if _, err := os.Stat(meterFilePath()); !os.IsNotExist(err) {
		t.Fatalf("无新数据时不应创建文件: %v", err)
	}
}
