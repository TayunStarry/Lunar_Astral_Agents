package thinking

// network_explore_tool_test.go 钉住联网探索的统一输入输出、分类与生活链触发响应。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

func TestNetworkExploreExposesLifeCategories(t *testing.T) {
	enum := networkExploreEnum(t)
	for _, want := range []string{"web_search", "bilibili_viewer", "mutual_forum"} {
		if !enumContains(t, enum, want) {
			t.Fatalf("联网探索应始终暴露 %s，实际 %v", want, enum)
		}
	}
}

func TestNetworkExploreLifeTriggersFixedReply(t *testing.T) {
	prepareExploreLifeDir(t)

	result := decodeExploreResult(t, Dialogue.ExecuteNetworkExplore(map[string]any{
		"type":        Dialogue.NetworkMutualForumTool,
		"instruction": "想看看大家在聊什么",
		"handle":      []any{},
		"details":     false,
	}))
	if result.Error != "" {
		t.Fatalf("互助论坛不应报错: %s", result.Error)
	}
	if result.Result != Dialogue.ForumFixedReply {
		t.Fatalf("互助论坛应返回固定响应 %q，实际 %q", Dialogue.ForumFixedReply, result.Result)
	}
	if !life.HasPendingEvent() {
		t.Fatal("互助论坛触发后应登记强制生活事件")
	}
}

// TestNetworkExploreBilibiliTriggersLifeEvent 观影回到生活链：
// 工具只登记 watch 强制事件，真正流程由生活链事件调用 viewer.WatchNextSegment 执行。
func TestNetworkExploreBilibiliTriggersLifeEvent(t *testing.T) {
	prepareExploreLifeDir(t)

	result := decodeExploreResult(t, Dialogue.ExecuteNetworkExplore(map[string]any{
		"type":        Dialogue.NetworkBilibiliTool,
		"instruction": "想看点猫咪视频",
		"handle":      []any{},
		"details":     false,
	}))
	if result.Error != "" {
		t.Fatalf("观影分支不应报错: %s", result.Error)
	}
	if result.Result != Dialogue.WatchFixedReply {
		t.Fatalf("观影分支应返回固定响应 %q，实际 %q", Dialogue.WatchFixedReply, result.Result)
	}
	if !life.HasPendingEvent() {
		t.Fatal("观影触发后应登记 watch 强制生活事件")
	}
}

func TestNetworkExploreUnknownType(t *testing.T) {
	result := decodeExploreResult(t, Dialogue.ExecuteNetworkExplore(map[string]any{
		"type":        "unknown",
		"instruction": "随便看看",
		"handle":      []any{},
	}))
	if !strings.Contains(result.Error, "web_search") {
		t.Fatalf("未知分支应给出可选分类，实际 %q", result.Error)
	}
}

// TestNetworkExploreRejectsMalformedInput 非规范输入必须直接返回格式错误。
func TestNetworkExploreRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"未知字段", map[string]any{"tool": "web_search", "instruction": "x"}, "不支持的字段"},
		{"type 类型错误", map[string]any{"type": 1, "instruction": "x"}, "type 必须是字符串"},
		{"handle 类型错误", map[string]any{"type": "web_search", "instruction": "x", "handle": "x"}, "handle 必须是字符串数组"},
		{"details 类型错误", map[string]any{"type": "web_search", "instruction": "x", "details": "true"}, "details 必须是布尔值"},
		{"缺 instruction", map[string]any{"type": "web_search", "handle": []any{}}, "instruction 不能为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := decodeExploreResult(t, Dialogue.ExecuteNetworkExplore(tc.args))
			if !strings.Contains(result.Error, tc.want) {
				t.Fatalf("错误应包含 %q，实际 %q", tc.want, result.Error)
			}
			if result.Result != "" {
				t.Fatalf("格式错误时 result 必须为空，实际 %q", result.Result)
			}
			if result.Handle == nil || len(result.Handle) != 0 {
				t.Fatalf("格式错误时 handle 必须为空数组，实际 %#v", result.Handle)
			}
		})
	}
}

// TestNetworkExploreDetailsHasNoSideEffect details=true 只返回职能描述，不登记生活事件。
func TestNetworkExploreDetailsHasNoSideEffect(t *testing.T) {
	prepareExploreLifeDir(t)

	result := decodeExploreResult(t, Dialogue.ExecuteNetworkExplore(map[string]any{
		"type":    Dialogue.NetworkBilibiliTool,
		"details": true,
		"handle":  []any{},
	}))
	if result.Error != "" {
		t.Fatalf("details 查询不应报错: %s", result.Error)
	}
	if !strings.Contains(result.Result, "哔哩观影") {
		t.Fatalf("details 应返回职能描述，实际 %q", result.Result)
	}
	if life.HasPendingEvent() {
		t.Fatal("details=true 不应登记任何生活事件")
	}
}

// prepareExploreLifeDir 准备隔离的生活状态目录。
func prepareExploreLifeDir(t *testing.T) {
	t.Helper()
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	life.Init()
	t.Cleanup(func() {
		// 用全新目录重新载入生活状态，清掉测试留下的待执行事件，避免污染其他用例。
		config.LocalDir = t.TempDir()
		life.Init()
		config.LocalDir = oldDir
	})
}

// decodeExploreResult 解析统一输出原语（network 测试内的别名）。
func decodeExploreResult(t *testing.T, result ToolResult) agentdispatch.Result {
	t.Helper()
	return decodeToolOutput(t, result)
}

func networkExploreEnum(t *testing.T) []string {
	t.Helper()
	tools := networkExploreToolCalls()
	if len(tools) != 1 {
		t.Fatalf("聚合工具数量应为 1，实际 %d", len(tools))
	}
	params, ok := tools[0].Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("工具参数 Schema 类型异常: %T", tools[0].Function.Parameters)
	}
	properties := params["properties"].(map[string]any)
	typeProp := properties["type"].(map[string]any)
	enum, ok := typeProp["enum"].([]string)
	if !ok {
		t.Fatalf("type 枚举类型异常: %T", typeProp["enum"])
	}
	return enum
}

func enumContains(t *testing.T, values []string, target string) bool {
	t.Helper()
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
