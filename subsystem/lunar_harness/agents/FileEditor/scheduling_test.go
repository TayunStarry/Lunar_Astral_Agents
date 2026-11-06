package FileEditor

// scheduling_test.go 统一调度入口的验收：参数校验、目标提取、能力边界。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/library"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// prepareScheduleDir 准备隔离的文档库与句柄库
func prepareScheduleDir(t *testing.T) {
	t.Helper()
	origLocal, origArchive := config.LocalDir, config.ArchiveDBPath
	_ = library.Close()
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	_ = resource.Close()
	t.Cleanup(func() {
		_ = resource.Close()
		_ = library.Close()
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
		{"未知类型", map[string]any{"type": "bad", "instruction": "x", "handle": []any{}}, "type 只能取"},
		{"缺 instruction", map[string]any{"type": ScheduleArchiveReader, "handle": []any{}}, "instruction 不能为空"},
		{"旧字段 target", map[string]any{"type": ScheduleArchiveReader, "instruction": "x", "target": "a.zip"}, "不支持的字段"},
		{"handle 类型错误", map[string]any{"type": ScheduleArchiveReader, "instruction": "x", "handle": "a.zip"}, "handle 必须是字符串数组"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scheduleTask(context.Background(), tc.args)
			if got.OK || !strings.Contains(got.Summary, tc.want) {
				t.Fatalf("提示应包含 %q，实际 %v %q", tc.want, got.OK, got.Summary)
			}
			decoded := decodeScheduleResult(t, got)
			if !strings.Contains(decoded.Error, tc.want) {
				t.Fatalf("统一输出 error 应包含 %q，实际 %q", tc.want, decoded.Error)
			}
		})
	}
}

// TestTaskSchedulingUnifiedOutput 输出只能是 {result, handle, error} 三个字段。
func TestTaskSchedulingUnifiedOutput(t *testing.T) {
	got := scheduleTask(context.Background(), map[string]any{
		"type":    ScheduleVision,
		"details": true,
		"handle":  []any{},
	})
	decoded := decodeScheduleResult(t, got)
	if decoded.Error != "" {
		t.Fatalf("details 查询不应报错: %s", decoded.Error)
	}
	if !strings.Contains(decoded.Result, "视觉理解") {
		t.Fatalf("details 应返回职能描述，实际 %q", decoded.Result)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(got.JSON()), &raw); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	if len(raw) != 3 {
		t.Fatalf("输出字段必须恰好为 result / handle / error，实际 %v", raw)
	}
	for _, key := range []string{"result", "handle", "error"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("输出缺少字段 %s：%v", key, raw)
		}
	}
}

// TestTaskSchedulingArchiveReader 归档解析支持 handle 与从 instruction 提取路径。
func TestTaskSchedulingArchiveReader(t *testing.T) {
	prepareScheduleDir(t)

	created := ArchiveOps.Create("/document/pkg.zip", []string{"/document/a.txt"})
	if !created.OK {
		t.Fatalf("准备压缩包失败: %s", created.Summary)
	}

	explicit := scheduleTask(context.Background(), map[string]any{
		"type": ScheduleArchiveReader, "instruction": "读取 a.txt", "handle": []any{created.Path},
	})
	decoded := decodeScheduleResult(t, explicit)
	if !explicit.OK || !strings.Contains(decoded.Result, "alpha") {
		t.Fatalf("归档解析应读取到条目内容: %v %q", explicit.OK, decoded.Result)
	}

	extracted := scheduleTask(context.Background(), map[string]any{
		"type": ScheduleArchiveReader, "instruction": "查看 unknown/pkg.zip 的结构", "handle": []any{},
	})
	decoded = decodeScheduleResult(t, extracted)
	if !extracted.OK || !strings.Contains(decoded.Result, "a.txt") {
		t.Fatalf("应从 instruction 提取压缩包路径: %v %q", extracted.OK, decoded.Result)
	}
}

// decodeScheduleResult 解析 task_scheduling 的统一输出原语。
func decodeScheduleResult(t *testing.T, got OpResult) agentdispatch.Result {
	t.Helper()
	var decoded agentdispatch.Result
	if err := json.Unmarshal([]byte(got.JSON()), &decoded); err != nil {
		t.Fatalf("调度输出不是统一原语: %v（%q）", err, got.JSON())
	}
	return decoded
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
