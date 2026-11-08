package SearchPlanner

// route_test.go 结构化工具调用的纯函数测试：工具参数解析、管线校验与失败行为。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// toolCall 构造一次决策工具调用
func toolCall(args string) agentbase.ToolCall {
	return agentbase.ToolCall{
		Type:     "function",
		Function: agentbase.ToolFunction{Name: decisionToolName, Arguments: args},
	}
}

// TestDecisionFromToolCall 解析与校验：合法决策保留，未知管线 / 低把握回落检索
func TestDecisionFromToolCall(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		pipeline string
		query    string
	}{
		{
			name:     "天气管线",
			args:     `{"pipeline":"weather","query":"北京天气","args":{"city":"北京"},"confidence":0.9}`,
			pipeline: PipelineWeather,
			query:    "北京天气",
		},
		{
			name:     "未知管线回落检索",
			args:     `{"pipeline":"translate","query":"你好","confidence":0.9}`,
			pipeline: PipelineSearch,
			query:    "你好",
		},
		{
			name:     "低把握回落检索",
			args:     `{"pipeline":"weather","query":"明天","confidence":0.3}`,
			pipeline: PipelineSearch,
			query:    "明天",
		},
		{
			name:     "缺省把握视为可信",
			args:     `{"pipeline":"weather","query":"杭州","args":{"city":"杭州"}}`,
			pipeline: PipelineWeather,
			query:    "杭州",
		},
	}
	for _, c := range cases {
		decision, err := decisionFromToolCall(toolCall(c.args))
		if err != nil {
			t.Fatalf("%s 解析失败: %v", c.name, err)
		}
		if decision.Pipeline != c.pipeline {
			t.Fatalf("%s 管线应为 %s，实得 %s", c.name, c.pipeline, decision.Pipeline)
		}
		if c.query != "" && decision.Query != c.query {
			t.Fatalf("%s 检索词应为 %q，实得 %q", c.name, c.query, decision.Query)
		}
	}
}

// TestDecisionFromToolCallFailures 参数缺失/非法 JSON 必须返回明确错误
func TestDecisionFromToolCallFailures(t *testing.T) {
	cases := []struct {
		name string
		args string
	}{
		{name: "空参数", args: ""},
		{name: "非法 JSON", args: "我不确定"},
		{name: "缺少管线", args: `{"query":"杭州"}`},
	}
	for _, c := range cases {
		if _, err := decisionFromToolCall(toolCall(c.args)); err == nil {
			t.Fatalf("%s 应返回错误", c.name)
		}
	}
}

// TestDecisionFromToolCallTruncates 超长检索词按上限截短
func TestDecisionFromToolCallTruncates(t *testing.T) {
	long := strings.Repeat("长", routeQueryMaxRunes+20)
	decision, err := decisionFromToolCall(toolCall(`{"pipeline":"search","query":"` + long + `"}`))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if runes := len([]rune(decision.Query)); runes != routeQueryMaxRunes {
		t.Fatalf("检索词应截到 %d 字，实得 %d", routeQueryMaxRunes, runes)
	}
}

// TestDecisionCall 只接受决策工具：正文输出、未知工具、未调用都要明确报错
func TestDecisionCall(t *testing.T) {
	valid := toolCall(`{"pipeline":"search","query":"杭州"}`)
	if _, err := decisionCall(agentbase.Reply{ToolCalls: []agentbase.ToolCall{valid}}); err != nil {
		t.Fatalf("合法工具调用不应报错: %v", err)
	}

	if _, err := decisionCall(agentbase.Reply{ToolCalls: []agentbase.ToolCall{{
		Function: agentbase.ToolFunction{Name: "other_tool", Arguments: "{}"},
	}}}); err == nil || !strings.Contains(err.Error(), "未知工具") {
		t.Fatalf("未知工具应明确报错，实得: %v", err)
	}

	if _, err := decisionCall(agentbase.Reply{Text: `{"pipeline":"search"}`}); err == nil || !strings.Contains(err.Error(), "正文") {
		t.Fatalf("正文输出应明确报错，实得: %v", err)
	}

	if _, err := decisionCall(agentbase.Reply{}); err == nil || !strings.Contains(err.Error(), "未调用") {
		t.Fatalf("未调用工具应明确报错，实得: %v", err)
	}
}

// TestDecisionToolSchema 工具 schema 必须声明决策工具名、必填字段与管线枚举
func TestDecisionToolSchema(t *testing.T) {
	tool := decisionTool()
	if tool.Function.Name != decisionToolName {
		t.Fatalf("工具名应为 %s，实得 %s", decisionToolName, tool.Function.Name)
	}
	if !strings.Contains(tool.Function.Description, "必须调用") {
		t.Fatalf("工具描述应要求模型必须调用: %s", tool.Function.Description)
	}
	enum, _ := tool.Function.Parameters["properties"].(map[string]any)["pipeline"].(map[string]any)["enum"].([]string)
	if len(enum) != len(routePipelines) {
		t.Fatalf("管线枚举数量应为 %d，实得 %d", len(routePipelines), len(enum))
	}
	required, _ := tool.Function.Parameters["required"].([]string)
	if len(required) == 0 || required[0] != "pipeline" {
		t.Fatalf("必填字段应包含 pipeline: %v", required)
	}
}

// TestRouteCandidates 注册表生成的候选列表必须包含全部管线
func TestRouteCandidates(t *testing.T) {
	text := routeCandidates()
	for _, pipeline := range routePipelines {
		if !strings.Contains(text, pipeline.ID) {
			t.Fatalf("候选列表缺少管线 %s:\n%s", pipeline.ID, text)
		}
	}
}

// TestWeatherCity 城市参数只认结构化 args.city，缺失时交给 IP 定位
func TestWeatherCity(t *testing.T) {
	cases := []struct {
		decision Decision
		want     string
	}{
		{Decision{Args: map[string]any{"city": "北京"}}, "北京"},
		{Decision{Query: "上海"}, ""},
		{Decision{}, ""},
	}
	for _, c := range cases {
		if got := WeatherCity(c.decision); got != c.want {
			t.Fatalf("城市应为 %q，实得 %q", c.want, got)
		}
	}
}
