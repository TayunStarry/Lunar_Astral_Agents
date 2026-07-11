package routing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/types"
)

// aiHTTPClient 专用于AI路由调用的HTTP客户端，超时时间较长以等待AI推理
var aiHTTPClient = &http.Client{Timeout: time.Duration(config.DefaultAIAPITimeout) * time.Second}

// routingSystemPrompt 构建AI路由的系统提示词
func routingSystemPrompt(availableGroups []int64) string {
	var sb strings.Builder
	sb.WriteString("你是一个消息路由助手。根据消息内容，智能判断应该发送到哪些QQ群聊。\n\n")
	sb.WriteString("可用群聊列表：\n")
	for _, gid := range availableGroups {
		sb.WriteString(fmt.Sprintf("- 群ID: %d\n", gid))
	}
	sb.WriteString("\n规则：\n")
	sb.WriteString("1. 分析消息内容，判断其适合发送到哪个或哪些群\n")
	sb.WriteString("2. 如果消息内容有明确的群聊指向性，只返回相关群ID\n")
	sb.WriteString("3. 如果是通用消息（如公告、广播），返回所有群ID\n")
	sb.WriteString("4. 返回一个JSON对象，格式为 {\"group_ids\": [群ID数组]}\n")
	sb.WriteString("5. 只返回JSON，不要包含任何其他文字或解释\n")
	return sb.String()
}

// AnalyzeMessageRoute 调用AI分析消息内容，返回应推送的目标群聊ID列表。
// 如果AI路由未启用或调用失败，返回 nil 和 error，调用方应回退到默认路由。
func AnalyzeMessageRoute(messageContent string, availableGroups []int64) ([]int64, error) {
	if !config.GetAIRoutingEnabled() {
		return nil, fmt.Errorf("AI路由未启用")
	}

	if len(availableGroups) == 0 {
		return nil, fmt.Errorf("没有可用的群组")
	}

	if messageContent == "" {
		return nil, fmt.Errorf("消息内容为空")
	}

	model := config.GetAIRoutingModel()
	apiURL := config.GetLunarCoreV1URL()

	reqBody := types.ChatCompletionRequest{
		Model: model,
		Messages: []types.ChatCompletionMessage{
			{Role: "system", Content: routingSystemPrompt(availableGroups)},
			{Role: "user", Content: messageContent},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("AI路由: 序列化请求失败: %v", err)
		return nil, err
	}

	logger.Info("AI路由: 正在调用 %s 分析消息 (长度=%d字符, 可用群组=%d个)",
		apiURL, len(messageContent), len(availableGroups))

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("AI路由: 创建请求失败: %v", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := aiHTTPClient.Do(req)
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Error("AI路由: API调用失败 (耗时=%v): %v", elapsed, err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("AI路由: 读取响应失败: %v", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("AI路由: API返回非200状态码 %d: %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("AI API返回状态码 %d", resp.StatusCode)
	}

	var chatResp types.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		logger.Error("AI路由: 解析响应JSON失败: %v, 原始响应: %s", err, string(respBody))
		return nil, err
	}

	if len(chatResp.Choices) == 0 {
		logger.Error("AI路由: 响应中没有choices")
		return nil, fmt.Errorf("AI响应为空")
	}

	aiContent := chatResp.Choices[0].Message.Content
	logger.Info("AI路由: 收到响应 (耗时=%v): %s", elapsed, aiContent)

	// 尝试从AI响应中提取JSON
	groupIDs, err := parseAIRoutingResponse(aiContent, availableGroups)
	if err != nil {
		logger.Error("AI路由: 解析路由判定失败: %v", err)
		return nil, err
	}

	logger.Info("AI路由: 判定目标群组: %v", groupIDs)
	return groupIDs, nil
}

// parseAIRoutingResponse 从AI的文本响应中提取群组ID列表。
// AI可能返回纯JSON，也可能在JSON外包裹了其他文字，需要智能提取。
func parseAIRoutingResponse(aiContent string, availableGroups []int64) ([]int64, error) {
	// 尝试直接解析整个响应
	var decision types.AIRoutingDecision
	if err := json.Unmarshal([]byte(aiContent), &decision); err == nil {
		return validateGroupIDs(decision.GroupIDs, availableGroups)
	}

	// 尝试提取JSON部分（AI可能在JSON外包裹了文字）
	jsonStart := strings.Index(aiContent, "{")
	jsonEnd := strings.LastIndex(aiContent, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := aiContent[jsonStart : jsonEnd+1]
		if err := json.Unmarshal([]byte(jsonStr), &decision); err == nil {
			return validateGroupIDs(decision.GroupIDs, availableGroups)
		}
	}

	// 尝试提取数组格式 [123, 456]
	arrStart := strings.Index(aiContent, "[")
	arrEnd := strings.LastIndex(aiContent, "]")
	if arrStart >= 0 && arrEnd > arrStart {
		arrStr := aiContent[arrStart : arrEnd+1]
		var groupIDs []int64
		if err := json.Unmarshal([]byte(arrStr), &groupIDs); err == nil {
			return validateGroupIDs(groupIDs, availableGroups)
		}
	}

	return nil, fmt.Errorf("无法从AI响应中提取有效的群组ID: %s", aiContent)
}

// validateGroupIDs 验证群组ID是否在可用列表中，过滤无效ID
func validateGroupIDs(groupIDs []int64, availableGroups []int64) ([]int64, error) {
	if len(groupIDs) == 0 {
		return nil, fmt.Errorf("AI返回的群组ID列表为空")
	}

	// 构建可用群组集合用于快速查找
	availableSet := make(map[int64]bool)
	for _, gid := range availableGroups {
		availableSet[gid] = true
	}

	var validIDs []int64
	for _, gid := range groupIDs {
		if availableSet[gid] {
			validIDs = append(validIDs, gid)
		} else {
			logger.Warn("AI路由: AI返回了不在可用列表中的群组ID %d，已过滤", gid)
		}
	}

	if len(validIDs) == 0 {
		return nil, fmt.Errorf("AI返回的所有群组ID均无效")
	}

	return validIDs, nil
}

// AnalyzeBatchMessages 批量分析消息并返回每条消息对应的目标群组。
// 每条消息独立调用AI判定，但通过并发控制避免API过载。
func AnalyzeBatchMessages(messages []string, availableGroups []int64) ([][]int64, []error) {
	results := make([][]int64, len(messages))
	errs := make([]error, len(messages))

	if !config.GetAIRoutingEnabled() {
		for i := range messages {
			errs[i] = fmt.Errorf("AI路由未启用")
		}
		return results, errs
	}

	// 逐条分析（保持顺序，避免并发过多导致API过载）
	for i, msg := range messages {
		groupIDs, err := AnalyzeMessageRoute(msg, availableGroups)
		if err != nil {
			errs[i] = err
			logger.Warn("AI路由: 第 %d/%d 条消息路由分析失败: %v", i+1, len(messages), err)
		} else {
			results[i] = groupIDs
		}
	}

	return results, errs
}