package WebSearch

// route_test.go 结构化路由的纯函数测试：JSON 解析、管线校验与参数兜底。

import (
	"strings"
	"testing"
)

// TestParseRouteDecision 解析与校验：合法决策保留，未知管线 / 低把握回落检索
func TestParseRouteDecision(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		pipeline string
		query    string
	}{
		{
			name:     "天气管线",
			raw:      `{"pipeline":"weather","query":"北京天气","args":{"city":"北京"},"confidence":0.9}`,
			pipeline: PipelineWeather,
			query:    "北京天气",
		},
		{
			name:     "代码块包裹",
			raw:      "```json\n{\"pipeline\":\"weather\",\"args\":{\"city\":\"上海\"},\"confidence\":0.8}\n```",
			pipeline: PipelineWeather,
		},
		{
			name:     "未知管线回落检索",
			raw:      `{"pipeline":"translate","query":"你好","confidence":0.9}`,
			pipeline: PipelineSearch,
			query:    "你好",
		},
		{
			name:     "低把握回落检索",
			raw:      `{"pipeline":"weather","query":"明天","confidence":0.3}`,
			pipeline: PipelineSearch,
			query:    "明天",
		},
		{
			name:     "缺省把握视为可信",
			raw:      `{"pipeline":"weather","query":"杭州","args":{"city":"杭州"}}`,
			pipeline: PipelineWeather,
			query:    "杭州",
		},
	}
	for _, c := range cases {
		decision, err := parseRouteDecision(c.raw)
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
	if _, err := parseRouteDecision("我不确定"); err == nil {
		t.Fatal("非 JSON 输出应报错，交给调用点回落检索")
	}
}

// TestParseRouteDecisionTruncates 超长检索词按上限截短
func TestParseRouteDecisionTruncates(t *testing.T) {
	long := strings.Repeat("长", routeQueryMaxRunes+20)
	decision, err := parseRouteDecision(`{"pipeline":"search","query":"` + long + `"}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if runes := len([]rune(decision.Query)); runes != routeQueryMaxRunes {
		t.Fatalf("检索词应截到 %d 字，实得 %d", routeQueryMaxRunes, runes)
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
		decision pipelineDecision
		want     string
	}{
		{pipelineDecision{Args: map[string]any{"city": "北京"}}, "北京"},
		{pipelineDecision{Query: "上海"}, ""},
		{pipelineDecision{}, ""},
	}
	for _, c := range cases {
		if got := weatherCity(c.decision); got != c.want {
			t.Fatalf("城市应为 %q，实得 %q", c.want, got)
		}
	}
}
