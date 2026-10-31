package WebSearch

// route.go 结构化路由：把「提炼检索词」升级为「一步决定管线 + 查询词 + 参数」。
//
// 搜索模型的同一次调用同时输出 pipeline / query / args / confidence；
// 新增直连能力只需往 variable.go 的 routePipelines 注册一条，无需新增正则或分支。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Route 调用搜索模型产出结构化决策；调用或解析失败时回退检索管线。
func Route(instruction string, profile summaryProfile) pipelineDecision {
	fallback := pipelineDecision{Pipeline: PipelineSearch, Query: instruction}
	system := loadPrompt("webSearchRoute.md")
	if system == "" {
		return fallback
	}
	system = renderPrompt(system, "{pipelines}", routeCandidates())
	text, err := chatWithOptions([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: instruction},
	}, profileChatOptions(profile))
	if err != nil {
		loggerWarn("路由模型调用失败，按检索管线处理: %v", err)
		return fallback
	}
	decision, err := parseRouteDecision(text)
	if err != nil {
		loggerWarn("路由输出解析失败，按检索管线处理: %v", err)
		return fallback
	}
	if strings.TrimSpace(decision.Query) == "" {
		decision.Query = instruction
	}
	return decision
}

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

// parseRouteDecision 解析并校验模型输出的 JSON 决策：
// 未知管线或把握不足时回落检索管线，检索词统一截到上限。
func parseRouteDecision(raw string) (pipelineDecision, error) {
	text := strings.TrimSpace(raw)
	// 兼容模型把 JSON 包在 ```json 代码块或说明文字里
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			text = text[start : end+1]
		}
	}
	var decision pipelineDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		return pipelineDecision{}, fmt.Errorf("不是合法的 JSON 决策: %w", err)
	}
	decision.Pipeline = strings.ToLower(strings.TrimSpace(decision.Pipeline))
	if !knownPipeline(decision.Pipeline) || decisionConfidence(decision) < routeConfidenceMin {
		decision.Pipeline = PipelineSearch
	}
	decision.Query = strings.Trim(strings.TrimSpace(decision.Query), "\"“”「」")
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
func decisionConfidence(decision pipelineDecision) float64 {
	if decision.Confidence == nil {
		return 1
	}
	return *decision.Confidence
}

// weatherCity 取天气管线需要的城市：只认结构化参数 args.city；
// 没有城市时返回空串，由 wttr.in 按请求方 IP 定位。
func weatherCity(decision pipelineDecision) string {
	if value, ok := decision.Args["city"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
