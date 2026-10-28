package debugsnap

// snapshot_test.go 钉住调试快照的契约：
// 开发者模式才写、目录结构完整、并发/冷却限流生效、智能体上下文汇总有效。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/agentdump"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
)

// withTempSnapshotEnv 准备隔离环境：临时 LocalDir、developer 开启、清空限流状态。
func withTempSnapshotEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	previousLocal := config.LocalDir
	previousDeveloper := config.Developer
	previousCooldown := snapshotCooldown

	config.LocalDir = root
	config.Developer = true
	snapshotCooldown = defaultCooldown
	lastSnapshotAt.Store(0)
	snapshotBusy.Store(false)

	t.Cleanup(func() {
		config.LocalDir = previousLocal
		config.Developer = previousDeveloper
		snapshotCooldown = previousCooldown
		lastSnapshotAt.Store(0)
		snapshotBusy.Store(false)
	})
	return root
}

// withFakeSources 用假取数实现替换真实数据源（避免依赖日志管线与模型环境）。
func withFakeSources(t *testing.T, fake snapshotSources) {
	t.Helper()
	previous := sources
	sources = fake
	t.Cleanup(func() { sources = previous })
}

// fakeSources 返回一组可计数的假数据源。
func fakeSources(calls *atomic.Int32) snapshotSources {
	return snapshotSources{
		rawLogs: func() string {
			calls.Add(1)
			return "{\"level\":\"WARN\",\"text\":\"synthetic warning\"}\n"
		},
		reasoning: func() (string, error) {
			return "{\"meta\":{\"turn_count\":1},\"turns\":[]}\n", nil
		},
		agents: func() (string, error) {
			return "{\"count\":0,\"agents\":[]}\n", nil
		},
	}
}

// snapshotDirs 列出快照根目录下的全部快照目录。
func snapshotDirs(t *testing.T, root string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "logs", "debug", "*"))
	if err != nil {
		t.Fatalf("扫描快照目录失败: %v", err)
	}
	dirs := make([]string, 0, len(matches))
	for _, path := range matches {
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return dirs
}

// readMeta 读取并解析 meta.json
func readMeta(t *testing.T, dir string) Meta {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		t.Fatalf("读取 meta.json 失败: %v", err)
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("解析 meta.json 失败: %v", err)
	}
	return meta
}

func TestHandleAlertWritesSnapshotInDeveloperMode(t *testing.T) {
	root := withTempSnapshotEnv(t)
	var calls atomic.Int32
	withFakeSources(t, fakeSources(&calls))

	HandleAlert("WARN", "TestModule", "Sub", "磁盘占用过高")
	snapshotWG.Wait()

	if calls.Load() != 1 {
		t.Fatalf("应执行 1 次取数，实际 %d 次", calls.Load())
	}
	dirs := snapshotDirs(t, root)
	if len(dirs) != 1 {
		t.Fatalf("应生成 1 个快照目录，实际 %d 个: %v", len(dirs), dirs)
	}

	dir := dirs[0]
	for _, name := range []string{rawLogsFile, reasoningFile, agentsFile, metaFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("快照缺少 %s: %v", name, err)
		}
	}

	meta := readMeta(t, dir)
	if meta.Kind != snapshotKind {
		t.Fatalf("meta.kind 不符: %q", meta.Kind)
	}
	if meta.Level != "WARN" || meta.Module != "TestModule" || meta.Sub != "Sub" {
		t.Fatalf("meta 触发信息不符: %+v", meta)
	}
	if meta.Message != "磁盘占用过高" {
		t.Fatalf("meta.message 不符: %q", meta.Message)
	}
	if !meta.Developer {
		t.Fatal("meta.developer 应为 true")
	}
	if meta.Version == "" {
		t.Fatal("meta.version 不应为空")
	}
	if len(meta.Files) != 4 {
		t.Fatalf("meta.files 应列出 4 个文件，实际: %v", meta.Files)
	}
	if _, err := time.Parse(time.RFC3339, meta.TriggeredAt); err != nil {
		t.Fatalf("meta.triggered_at 不是 RFC3339: %q", meta.TriggeredAt)
	}

	raw, err := os.ReadFile(filepath.Join(dir, rawLogsFile))
	if err != nil {
		t.Fatalf("读取 raw-logs.log 失败: %v", err)
	}
	if !strings.Contains(string(raw), "synthetic warning") {
		t.Fatalf("raw-logs.log 内容不符: %s", raw)
	}
}

func TestHandleAlertSkipsWhenDeveloperDisabledOrLevelTooLow(t *testing.T) {
	root := withTempSnapshotEnv(t)
	var calls atomic.Int32
	withFakeSources(t, fakeSources(&calls))

	config.Developer = false
	HandleAlert("ERROR", "TestModule", "", "非开发者模式")
	snapshotWG.Wait()
	if calls.Load() != 0 {
		t.Fatalf("非开发者模式不应取数，实际 %d 次", calls.Load())
	}

	config.Developer = true
	HandleAlert("INFO", "TestModule", "", "级别不足")
	snapshotWG.Wait()
	if calls.Load() != 0 {
		t.Fatalf("INFO 级别不应取数，实际 %d 次", calls.Load())
	}

	if dirs := snapshotDirs(t, root); len(dirs) != 0 {
		t.Fatalf("不应生成快照目录: %v", dirs)
	}
}

func TestRateLimitAllowsOneSnapshotPerCooldown(t *testing.T) {
	root := withTempSnapshotEnv(t)
	var calls atomic.Int32
	withFakeSources(t, fakeSources(&calls))
	snapshotCooldown = time.Hour

	HandleAlert("WARN", "TestModule", "", "第一条")
	HandleAlert("ERROR", "TestModule", "", "冷却期内第二条")
	snapshotWG.Wait()

	if calls.Load() != 1 {
		t.Fatalf("冷却期内应只取数 1 次，实际 %d 次", calls.Load())
	}
	if dirs := snapshotDirs(t, root); len(dirs) != 1 {
		t.Fatalf("冷却期内应只有 1 个快照目录，实际: %v", dirs)
	}

	// 模拟冷却期结束：清空上次触发时刻后应能再次生成
	lastSnapshotAt.Store(0)
	HandleAlert("WARN", "TestModule", "", "冷却期结束后")
	snapshotWG.Wait()

	if calls.Load() != 2 {
		t.Fatalf("冷却结束后应累计取数 2 次，实际 %d 次", calls.Load())
	}
	if dirs := snapshotDirs(t, root); len(dirs) != 2 {
		t.Fatalf("冷却结束后应有 2 个快照目录，实际: %v", dirs)
	}
}

func TestSnapshotDirectoriesDoNotCollide(t *testing.T) {
	withTempSnapshotEnv(t)
	var calls atomic.Int32
	withFakeSources(t, fakeSources(&calls))

	at := time.Now()
	first, err := writeSnapshot(Alert{Level: "WARN", Module: "TestModule", Message: "一", At: at})
	if err != nil {
		t.Fatalf("第一次写快照失败: %v", err)
	}
	second, err := writeSnapshot(Alert{Level: "WARN", Module: "TestModule", Message: "二", At: at})
	if err != nil {
		t.Fatalf("第二次写快照失败: %v", err)
	}
	if first == second {
		t.Fatalf("同一秒内的两次快照不应写入同一目录: %s", first)
	}
}

func TestCollectAgentsContextIncludesRegisteredProviders(t *testing.T) {
	const id = "debugsnap_test_provider"
	agentdump.Register(agentdump.Provider{
		ID:    id,
		Label: "快照测试目标",
		Group: "测试",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{"message_count": 1, "include_images": includeImages}, nil
		},
	})

	content, err := collectAgentsContext()
	if err != nil {
		t.Fatalf("汇总智能体上下文失败: %v", err)
	}
	var payload AgentsContext
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("汇总内容不是合法 JSON: %v\n%s", err, content)
	}
	found := false
	for _, entry := range payload.Agents {
		if entry.ID != id {
			continue
		}
		found = true
		if entry.Error != "" {
			t.Fatalf("测试 provider 不应导出失败: %s", entry.Error)
		}
		var inner struct {
			MessageCount int `json:"message_count"`
		}
		if err := json.Unmarshal(entry.Content, &inner); err != nil {
			t.Fatalf("条目内容不是合法 JSON: %v", err)
		}
		if inner.MessageCount != 1 {
			t.Fatalf("条目内容不符: %+v", inner)
		}
	}
	if !found {
		t.Fatalf("汇总结果缺少已注册的 %s: %s", id, content)
	}
}

func TestCollectRawLogsUsesRawBuffer(t *testing.T) {
	console.ClearLogs(console.SourceRaw)
	t.Cleanup(func() { console.ClearLogs(console.SourceRaw) })
	console.PushTaskLog("[WARN] 快照原始日志测试")

	content := collectRawLogs()
	if !strings.Contains(content, "快照原始日志测试") {
		t.Fatalf("原始日志导出缺少测试行: %q", content)
	}
	if !strings.HasSuffix(content, "\n") {
		t.Fatalf("原始日志应按 JSON Lines 逐行换行: %q", content)
	}
}
