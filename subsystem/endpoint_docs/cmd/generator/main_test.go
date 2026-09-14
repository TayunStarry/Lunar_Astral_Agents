package main

import (
	"os"
	"path/filepath"
	"testing"

	EndpointDocs "LunarSubsystem/EndpointDocs"
)

// registrySource 模拟项目中的端点注册表源码
const registrySource = `package main

import "net/http"

func handlerA(w http.ResponseWriter, r *http.Request) {}

var SystemEndpoints = []SystemEndpoint{
	{Path: "/load/application", Handler: handlerA, Method: "POST", Description: "加载应用"},
	{Path: "/background", Handler: handlerA, Method: "GET", Description: "随机背景图片"},
}

// 其余变量不应被采集
var proxyPrefixes = []string{"/v1/"}
`

func writeTempSource(t *testing.T, source string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "variable.go")
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatalf("写临时源文件失败: %v", err)
	}
	return file
}

func TestExtractEndpoints(t *testing.T) {
	file := writeTempSource(t, registrySource)

	endpoints, err := extractEndpoints(file, "SystemEndpoints")
	if err != nil {
		t.Fatalf("提取失败: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("期望提取 2 个端点，实际 %d 个: %+v", len(endpoints), endpoints)
	}

	first := endpoints[0]
	if first.Path != "/load/application" || first.Method != "POST" || first.Description != "加载应用" {
		t.Fatalf("第一个端点字段不符: %+v", first)
	}
	if first.Handler != "handlerA" {
		t.Fatalf("Handler 名不符: %q", first.Handler)
	}
	if endpoints[1].Path != "/background" {
		t.Fatalf("端点顺序不符: %+v", endpoints[1])
	}
}

func TestExtractEndpointsMissingVar(t *testing.T) {
	file := writeTempSource(t, "package main\n\nvar SomethingElse = 1\n")

	if _, err := extractEndpoints(file, "SystemEndpoints"); err == nil {
		t.Fatal("注册表变量不存在时应报错")
	}
}

func TestExtractEndpointsRejectsEntryWithoutPath(t *testing.T) {
	source := `package main

var SystemEndpoints = []SystemEndpoint{
	{Handler: nil, Method: "GET", Description: "缺少 Path"},
}
`
	file := writeTempSource(t, source)

	if _, err := extractEndpoints(file, "SystemEndpoints"); err == nil {
		t.Fatal("缺少 Path 的端点应报错")
	}
}

func TestExtraFlagsParsing(t *testing.T) {
	var extras extraFlags
	if err := extras.Set("WS|/ws|工作室 WebSocket 消息通道"); err != nil {
		t.Fatalf("合法 extra 解析失败: %v", err)
	}
	if len(extras) != 1 || extras[0].Method != "WS" || extras[0].Path != "/ws" || extras[0].Description != "工作室 WebSocket 消息通道" {
		t.Fatalf("extra 字段不符: %+v", extras)
	}
	if err := extras.Set("缺少分隔符"); err == nil {
		t.Fatal("格式非法的 extra 应报错")
	}
}

func TestContainsEndpoint(t *testing.T) {
	endpoints := []EndpointDocs.Endpoint{{Path: "/api-docs"}, {Path: "/ping"}}
	if !containsEndpoint(endpoints, "/api-docs") {
		t.Fatal("/api-docs 应已存在")
	}
	if containsEndpoint(endpoints, "/ws") {
		t.Fatal("/ws 不应存在")
	}
}

func TestBuildPaths(t *testing.T) {
	endpoints := []EndpointDocs.Endpoint{
		{Path: "/simple", Method: "GET", Description: "简单端点", Handler: "simpleHandler"},
		{Path: "/multi", Method: "GET/POST/DELETE", Description: "组合方法", Handler: "multiHandler"},
		{Path: "/wild", Method: "ANY", Description: "任意方法", Handler: "wildHandler"},
		{Path: "/channel", Method: "WS", Description: "消息通道", Handler: "wsHandler"},
	}
	paths := EndpointDocs.BuildPaths(endpoints)

	if len(paths) != 4 {
		t.Fatalf("期望 4 个路径，实际 %d 个", len(paths))
	}

	simple := paths["/simple"]["get"]
	if simple.Summary != "简单端点" || simple.OperationID != "simpleHandler" || simple.XHandler != "simpleHandler" {
		t.Fatalf("单方法端点操作不符: %+v", simple)
	}
	if simple.XMethod != "" {
		t.Fatalf("单方法端点不应带 x-method: %q", simple.XMethod)
	}

	multi := paths["/multi"]
	if len(multi) != 3 {
		t.Fatalf("组合方法应展开为 3 个操作，实际 %d 个", len(multi))
	}
	for _, key := range []string{"get", "post", "delete"} {
		operation := multi[key]
		if operation.XMethod != "GET/POST/DELETE" {
			t.Fatalf("组合方法应保留原始方法串: %s → %+v", key, operation)
		}
		if operation.OperationID != "multiHandler_"+key {
			t.Fatalf("展开操作应追加方法后缀保证唯一: %s → %q", key, operation.OperationID)
		}
	}

	wild := paths["/wild"]
	if len(wild) != 7 {
		t.Fatalf("ANY 应展开为 7 个标准方法，实际 %d 个", len(wild))
	}
	if wild["post"].XMethod != "ANY" {
		t.Fatalf("ANY 展开应保留原始方法串: %+v", wild["post"])
	}

	channel := paths["/channel"]["get"]
	if !channel.XWebsocket || channel.XMethod != "WS" {
		t.Fatalf("WS 端点应带 x-websocket 与 x-method 标记: %+v", channel)
	}
}
