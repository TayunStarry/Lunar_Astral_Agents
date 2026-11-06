package activity

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// resetActivity 隔离测试：临时 LocalDir + 清空内存状态（模拟冷启动）。
func resetActivity(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	mu.Lock()
	state = State{}
	loaded = false
	mu.Unlock()
	t.Cleanup(func() {
		config.LocalDir = oldDir
		mu.Lock()
		state = State{}
		loaded = false
		mu.Unlock()
	})
}

func TestDefaultWhenUnset(t *testing.T) {
	resetActivity(t)
	if got := Current(); got != defaultText {
		t.Fatalf("未设置时应返回默认描述，实际 %q", got)
	}
	line := Line()
	if !strings.HasPrefix(line, fieldPrefix) {
		t.Fatalf("固定字段前缀缺失: %q", line)
	}
	if line == fieldPrefix {
		t.Fatalf("固定字段的描述不应为空: %q", line)
	}
}

func TestSetAndCurrent(t *testing.T) {
	resetActivity(t)
	Set("  在整理今天的观影笔记  ")
	if got := Current(); got != "在整理今天的观影笔记" {
		t.Fatalf("Current 应返回去除首尾空白后的描述，实际 %q", got)
	}
	if got, want := Line(), fieldPrefix+"在整理今天的观影笔记"; got != want {
		t.Fatalf("Line 渲染不符：want %q, got %q", want, got)
	}
	// 空描述回退为默认值，不留下空字段。
	Set("   ")
	if got := Current(); got != defaultText {
		t.Fatalf("空描述应回退默认值，实际 %q", got)
	}
}

func TestPersistsAcrossRestart(t *testing.T) {
	resetActivity(t)
	Set("在准备下一期创作选题")
	// 模拟进程重启：只清内存，保留 LocalDir 下的状态文件。
	mu.Lock()
	state = State{}
	loaded = false
	mu.Unlock()
	if got := Current(); got != "在准备下一期创作选题" {
		t.Fatalf("重启后应读到上次的值，实际 %q", got)
	}
}

func TestConcurrentSetAndCurrent(t *testing.T) {
	resetActivity(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(index int) {
			defer wg.Done()
			for round := 0; round < 10; round++ {
				Set(fmt.Sprintf("并发活动-%d-%d", index, round))
			}
		}(i)
		go func() {
			defer wg.Done()
			for round := 0; round < 50; round++ {
				if got := Current(); got == "" {
					t.Errorf("并发读取不应得到空描述")
					return
				}
				if line := Line(); !strings.HasPrefix(line, fieldPrefix) {
					t.Errorf("并发渲染缺少固定前缀: %q", line)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := Current(); got == "" {
		t.Fatal("并发写入后状态不应为空")
	}
}
