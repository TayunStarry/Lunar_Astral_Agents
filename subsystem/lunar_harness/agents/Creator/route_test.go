package Creator

// route_test.go 任务自动路由的验收：压缩包读取 / 写入、大型目录读取、普通文本文件三分支，
// 以及路由后的真实读取能力（不依赖模型）。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/ArchiveOps"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// prepareRouteDir 准备路由测试用的文档库。
func prepareRouteDir(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
	dir := filepath.Join(config.LocalDir, "documents")
	for _, sub := range []string{"", "folderSmall", "folderLarge"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte("短文本"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte(strings.Repeat("长文本", largeFileBytes/3)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "folderSmall", "one.md"), []byte("# 小目录文件"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 文件数超过 largeFolderFiles 即判定为大型目录
	for index := 0; index < largeFolderFiles+5; index++ {
		name := filepath.Join(dir, "folderLarge", fmt.Sprintf("note-%02d.md", index))
		if err := os.WriteFile(name, []byte(fmt.Sprintf("# 第 %d 篇\n正文内容", index)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	created := ArchiveOps.Create("/document/pkg.zip", []string{"/document/small.txt"})
	if !created.OK {
		t.Fatalf("准备压缩包失败: %s", created.Summary)
	}
}

// TestRouteDecisionsCoverThreeBranches 路由按目标实际扩展名与目录规模判定。
func TestRouteDecisionsCoverThreeBranches(t *testing.T) {
	prepareRouteDir(t)
	cases := []struct {
		name   string
		target string
		op     string
		want   routeKind
	}{
		{"压缩包读取", "/document/pkg.zip", fileflow.OpRead, routeArchiveReader},
		{"压缩包写入", "/document/pkg.zip", fileflow.OpWrite, routeArchiveOps},
		{"大型目录读取", "/document/folderLarge", fileflow.OpRead, routeDocumentReader},
		{"小型目录读取", "/document/folderSmall", fileflow.OpRead, routeFileEditor},
		{"大文件读取", "/document/large.txt", fileflow.OpRead, routeDocumentReader},
		{"小文件读取", "/document/small.txt", fileflow.OpRead, routeFileEditor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := routeTarget(tc.target, tc.op)
			if err != nil {
				t.Fatalf("路由失败: %v", err)
			}
			if decision.Kind != tc.want {
				t.Fatalf("路由结果 = %s，期望 %s（依据：%s）", decision.Kind, tc.want, decision.Reason)
			}
		})
	}
}

// TestRoutedReadsUseCapabilityAgents 路由后的读取由对应能力智能体真实完成。
func TestRoutedReadsUseCapabilityAgents(t *testing.T) {
	prepareRouteDir(t)
	s := &session{mode: fileflow.ModeAssigned, flowID: "route-test"}
	ctx := context.Background()

	archive := decodeFlowOutcome(t, s.fileRead(ctx, map[string]any{"path": "/document/pkg.zip"}))
	if route, _ := archive["route"].(string); route != string(routeArchiveReader) {
		t.Fatalf("压缩包读取应由归档解析承接: %#v", archive)
	}
	if text, _ := archive["result"].(string); !strings.Contains(text, "small.txt") {
		t.Fatalf("归档解析结果应包含条目名: %q", text)
	}

	folder := decodeFlowOutcome(t, s.fileList(ctx, map[string]any{"path": "/document/folderLarge"}))
	if route, _ := folder["route"].(string); route != string(routeDocumentReader) {
		t.Fatalf("大型目录读取应由文献解析承接: %#v", folder)
	}
	if text, _ := folder["result"].(string); !strings.Contains(text, "【目录】") {
		t.Fatalf("文献解析应返回目录摘要: %q", text)
	}

	file := decodeFlowOutcome(t, s.fileRead(ctx, map[string]any{"path": "/document/small.txt"}))
	if op, _ := file["op"].(string); op != "file_read" {
		t.Fatalf("普通文本读取应由文件编辑承接: %#v", file)
	}
}

// TestPlannerOnlyAllowsReadTools 流程编排阶段只能读取证，写操作必须进入计划步骤。
func TestPlannerOnlyAllowsReadTools(t *testing.T) {
	for _, tool := range []string{toolFileCreate, toolFileRewrite, toolFileDelete, toolArchiveWrite, toolRunScript} {
		if plannerToolAllowed(tool) {
			t.Fatalf("调度阶段不应允许写操作工具 %s", tool)
		}
	}
	for _, tool := range []string{toolFileRead, toolFileList, toolArchiveRead} {
		if !plannerToolAllowed(tool) {
			t.Fatalf("调度阶段应允许只读工具 %s", tool)
		}
	}
}

// decodeFlowOutcome 解析流程工具回填的 JSON。
func decodeFlowOutcome(t *testing.T, outcome toolOutcome) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(outcome.JSON), &decoded); err != nil {
		t.Fatalf("工具回填不是合法 JSON: %v（%q）", err, outcome.JSON)
	}
	return decoded
}
