package thinking

// fetch_resources_test.go 统一资源工具的入参规范与校验路径。

import (
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// TestFetchResourcesValidation 钉住三种模式的必填校验与非法类型提示。
func TestFetchResourcesValidation(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"非法 type", map[string]any{"type": "bad"}, "type 只能取 memory / handle / path / list"},
		{"memory 缺 query", map[string]any{"type": "memory"}, "query 需要 1-4 个搜索关键词"},
		{"handle 缺 query", map[string]any{"type": "handle"}, "query 需要句柄"},
		{"path 缺 query", map[string]any{"type": "path"}, "query 需要文件名或相对路径"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := handleFetchResources(tc.args).Text
			if !strings.Contains(got, tc.want) {
				t.Fatalf("提示应包含 %q，实际 %q", tc.want, got)
			}
		})
	}
}

// TestFetchResourceToolShape 钉住工具定义：type/query/limit 三个参数，type 只有四个取值。
func TestFetchResourceToolShape(t *testing.T) {
	calls := fetchResourceToolCalls()
	if len(calls) != 1 {
		t.Fatalf("应只暴露一个资源工具，实际 %d 个", len(calls))
	}
	def := calls[0].Function
	if def.Name != "fetch_resources" {
		t.Fatalf("工具名应为 fetch_resources，实际 %q", def.Name)
	}
	params, ok := def.Parameters.(map[string]any)
	if !ok {
		t.Fatal("parameters 不是对象定义")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("缺少 properties 定义")
	}
	for _, key := range []string{"type", "query", "limit"} {
		if _, exists := props[key]; !exists {
			t.Fatalf("缺少参数 %q", key)
		}
	}
	typeDef, _ := props["type"].(map[string]any)
	enum, _ := typeDef["enum"].([]string)
	if len(enum) != 4 || enum[0] != "memory" || enum[1] != "handle" || enum[2] != "path" || enum[3] != "list" {
		t.Fatalf("type 枚举应为 memory/handle/path/list，实际 %v", enum)
	}
	required, _ := params["required"].([]string)
	if len(required) != 1 || required[0] != "type" {
		t.Fatalf("必填项应只有 type，实际 %v", required)
	}
}

// TestFetchResourcesListAndHandleEmpty 用空资源库钉住 list / handle 两种模式的真实返回。
func TestFetchResourcesListAndHandleEmpty(t *testing.T) {
	origDir, origDB := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	resource.Close()
	t.Cleanup(func() {
		resource.Close()
		config.LocalDir, config.ArchiveDBPath = origDir, origDB
	})

	list := handleFetchResources(map[string]any{"type": "list", "query": []any{"I"}, "limit": float64(5)}).Text
	if !strings.Contains(list, "资源目录为空") {
		t.Fatalf("空库 list 应返回空目录提示，实际 %q", list)
	}

	missing := handleFetchResources(map[string]any{"type": "handle", "query": []any{"@D.001"}}).Text
	if !strings.Contains(missing, "取回失败") || !strings.Contains(missing, "fetch_resources") {
		t.Fatalf("缺失句柄应提示用 fetch_resources 确认，实际 %q", missing)
	}
}

// TestFetchResourcesListFilters 钉住 list 模式的名称 / 文件夹 / 路径过滤。
func TestFetchResourcesListFilters(t *testing.T) {
	origDir, origDB := config.LocalDir, config.ArchiveDBPath
	config.LocalDir = t.TempDir()
	config.ArchiveDBPath = filepath.Join(config.LocalDir, "database", "handles.db")
	_ = resource.Close()
	t.Cleanup(func() {
		_ = resource.Close()
		config.LocalDir, config.ArchiveDBPath = origDir, origDB
	})
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		t.Fatalf("初始化句柄库失败: %v", err)
	}

	admitted, err := resource.Admit(resource.AdmitInput{
		SourceKind: resource.SourceKindBytes,
		Data:       []byte("# 前端资源库\n常用前端依赖清单。\n"),
		Name:       "06-前端资源库.md",
		Origin:     resource.OriginClientUpload,
	})
	if err != nil || admitted.Handle == "" {
		t.Fatalf("入库失败: %v", err)
	}

	cases := [][]any{
		{"06-前端资源库.md"},           // 纯文件名
		{"前端资源库"},                 // 文件名片段
		{"documents"},             // 纯文件夹名
		{"documents/06-前端资源库.md"}, // 相对路径
		{`documents\06-前端资源库.md`}, // Windows 反斜杠路径
		{"D", "06-前端资源库.md"},      // 类型码 + 文件名
	}
	for _, query := range cases {
		got := handleFetchResources(map[string]any{"type": "list", "query": query}).Text
		if !strings.Contains(got, admitted.Handle) {
			t.Fatalf("query=%v 应列出句柄 %s，实际 %q", query, admitted.Handle, got)
		}
	}

	miss := handleFetchResources(map[string]any{"type": "list", "query": []any{"不存在.md"}}).Text
	if strings.Contains(miss, admitted.Handle) || !strings.Contains(miss, "没有匹配") {
		t.Fatalf("无匹配时应提示没有匹配，实际 %q", miss)
	}

	// handle 模式只认句柄：文件名应被明确拦截并提示改用 path
	read := handleFetchResources(map[string]any{
		"type": "handle", "query": []any{"06-前端资源库.md"},
	}).Text
	if !strings.Contains(read, "不是句柄") || !strings.Contains(read, "type=\"path\"") {
		t.Fatalf("handle 模式应拒绝文件名并提示改用 path，实际 %q", read)
	}

	// path 模式按文件名 / 路径解析为句柄后取回
	byPath := handleFetchResources(map[string]any{
		"type": "path", "query": []any{"documents/06-前端资源库.md"},
	}).Text
	if !strings.Contains(byPath, admitted.Handle) || !strings.Contains(byPath, "常用前端依赖清单") {
		t.Fatalf("path 模式应按路径解析句柄并取回原文，实际 %q", byPath)
	}
}
