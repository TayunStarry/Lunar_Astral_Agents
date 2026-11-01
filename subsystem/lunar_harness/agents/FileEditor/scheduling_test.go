package FileEditor

// scheduling_test.go 统一调度入口的验收：参数校验、目标提取、能力边界。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// prepareScheduleDir 准备隔离的文档库与句柄库
func prepareScheduleDir(t *testing.T) {
	t.Helper()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	_ = resource.Close()
	t.Cleanup(func() {
		_ = resource.Close()
		config.LocalDir, config.ArchiveDBPath = origLocal, origArchive
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化句柄库失败: %v", err)
	}
	dir := filepath.Join(config.LocalDir, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTaskSchedulingValidation 参数校验必须先于调度发生。
func TestTaskSchedulingValidation(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"未知类型", map[string]any{"type": "bad", "instruction": "x"}, "type 只能取"},
		{"缺 instruction", map[string]any{"type": ScheduleArchiveReader}, "instruction 不能为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scheduleTask(context.Background(), tc.args)
			if got.OK || !strings.Contains(got.Summary, tc.want) {
				t.Fatalf("提示应包含 %q，实际 %v %q", tc.want, got.OK, got.Summary)
			}
		})
	}
}

// TestTaskSchedulingArchiveReader 归档解析支持 target 与从 instruction 提取路径。
func TestTaskSchedulingArchiveReader(t *testing.T) {
	prepareScheduleDir(t)

	created := ArchiveOps.Create("/document/pkg.zip", []string{"/document/a.txt"})
	if !created.OK {
		t.Fatalf("准备压缩包失败: %s", created.Summary)
	}

	explicit := scheduleTask(context.Background(), map[string]any{
		"type": ScheduleArchiveReader, "target": created.Path, "instruction": "读取 a.txt",
	})
	if !explicit.OK || !strings.Contains(explicit.Summary, "alpha") {
		t.Fatalf("归档解析应读取到条目内容: %v %q", explicit.OK, explicit.Summary)
	}

	extracted := scheduleTask(context.Background(), map[string]any{
		"type": ScheduleArchiveReader, "instruction": "查看 unknown/pkg.zip 的结构",
	})
	if !extracted.OK || !strings.Contains(extracted.Summary, "a.txt") {
		t.Fatalf("应从 instruction 提取压缩包路径: %v %q", extracted.OK, extracted.Summary)
	}
}

// TestFileToolsRejectArchivePath 普通文件工具指向压缩包时必须引导走调度入口。
func TestFileToolsRejectArchivePath(t *testing.T) {
	prepareScheduleDir(t)

	got := dispatchTool(context.Background(), ToolRead, map[string]any{"path": "unknown/pkg.zip"})
	if got.OK || !strings.Contains(got.Summary, ToolSchedule) {
		t.Fatalf("压缩包不应被 file_read 直接读取: %v %q", got.OK, got.Summary)
	}
	guarded := dispatchTool(context.Background(), ToolRead, map[string]any{"path": "notes/a.md"})
	if !guarded.OK && strings.Contains(guarded.Summary, ToolSchedule) {
		t.Fatalf("普通文档不应触发压缩包引导: %q", guarded.Summary)
	}
}
