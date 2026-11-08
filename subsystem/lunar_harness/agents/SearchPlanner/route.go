package SearchPlanner

// route.go 结构化决策的读取与校验（管线注册表驱动，新增管线无需改解析逻辑）。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// routeCandidates 由注册表生成提示词里的候选管线列表
func routeCandidates() string {
	var builder strings.Builder
	for index, pipeline := range routePipelines {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(pipeline.ID)
		builder.WriteString("：")
		builder.WriteString(pipeline.Description)
	}
	return builder.String()
}

// decisionCall 从模型响应中取决策工具调用；正文输出、未知工具、未调用都返回明确错误。
func decisionCall(reply agentbase.Reply) (agentbase.ToolCall, error) {
	for _, call := range reply.ToolCalls {
		if call.Function.Name == decisionToolName {
			return call, nil
		}
	}
	if len(reply.ToolCalls) > 0 {
		return agentbase.ToolCall{}, fmt.Errorf("模型调用了未知工具 %q（期望 %s）", reply.ToolCalls[0].Function.Name, decisionToolName)
	}
	if strings.TrimSpace(reply.Text) != "" {
		return agentbase.ToolCall{}, fmt.Errorf("模型未调用 %s 工具，而是用正文输出（正文已忽略）", decisionToolName)
	}
	return agentbase.ToolCall{}, fmt.Errorf("模型未调用 %s 工具", decisionToolName)
}

// decisionFromToolCall 解析并校验工具参数：
// 未知管线或把握不足时回落检索管线，检索词统一截到上限。
func decisionFromToolCall(call agentbase.ToolCall) (Decision, error) {
	raw := strings.TrimSpace(call.Function.Arguments)
	if raw == "" {
		return Decision{}, fmt.Errorf("工具参数为空")
	}
	var decision Decision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return Decision{}, fmt.Errorf("工具参数不是合法 JSON: %w", err)
	}

	decision.Pipeline = strings.ToLower(strings.TrimSpace(decision.Pipeline))
	if decision.Pipeline == "" {
		return Decision{}, fmt.Errorf("工具参数缺少 pipeline")
	}
	if !knownPipeline(decision.Pipeline) {
		logger.Warn("CrystalAstral", "路由工具返回未知管线 %q，按检索管线处理", decision.Pipeline)
		decision.Pipeline = PipelineSearch
	} else if decisionConfidence(decision) < routeConfidenceMin {
		decision.Pipeline = PipelineSearch
	}

	decision.Query = strings.Trim(strings.TrimSpace(decision.Query), "\"“”「」")
	if decision.Query == "" {
		logger.Warn("CrystalAstral", "路由工具未给出 query，将按用户指令检索")
	}
	if runes := []rune(decision.Query); len(runes) > routeQueryMaxRunes {
		decision.Query = strings.TrimSpace(string(runes[:routeQueryMaxRunes]))
	}
	return decision, nil
}

// knownPipeline 判断管线是否在注册表内
func knownPipeline(id string) bool {
	for _, pipeline := range routePipelines {
		if pipeline.ID == id {
			return true
		}
	}
	return false
}

// decisionConfidence 未给出把握时按「可信」处理
func decisionConfidence(decision Decision) float64 {
	if decision.Confidence == nil {
		return 1
	}
	return *decision.Confidence
}

// WeatherCity 取天气管线需要的城市：只认结构化参数 args.city；
// 没有城市时返回空串，由 wttr.in 按请求方 IP 定位。
func WeatherCity(decision Decision) string {
	if value, ok := decision.Args["city"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
