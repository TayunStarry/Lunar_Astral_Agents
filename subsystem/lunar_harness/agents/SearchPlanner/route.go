package SearchPlanner

// route.go 结构化决策的解析与校验（管线注册表驱动，新增管线无需改解析逻辑）。

import (
	"encoding/json"
	"fmt"
	"strings"
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

// parseDecision 解析并校验模型输出的 JSON 决策：
// 未知管线或把握不足时回落检索管线，检索词统一截到上限。
func parseDecision(raw string) (Decision, error) {
	text := strings.TrimSpace(raw)
	// 兼容模型把 JSON 包在 ```json 代码块或说明文字里
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			text = text[start : end+1]
		}
	}
	var decision Decision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		return Decision{}, fmt.Errorf("不是合法的 JSON 决策: %w", err)
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
