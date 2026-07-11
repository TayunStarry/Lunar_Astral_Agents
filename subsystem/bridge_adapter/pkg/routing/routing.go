package routing

// AI 路由引擎：基于群聊摘要 + 消息内容智能判定推送目标

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bridge_adapter/pkg/cache"
	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/types"
)

// RouteContext 路由上下文，包含来源群聊、可用群列表、消息类型
type RouteContext struct {
	SourceGroupID   int64   // 消息来源群聊ID
	AvailableGroups []int64 // 所有可用的群聊ID列表
	MsgType         string  // 消息类型：response/active/context/image（仅作类型参考）
}

// AnalyzeMessageRoute 调用AI分析消息内容，返回应推送的目标群聊ID列表。
// 所有消息类型均通过AI路由判定，response/active/context/image仅作为类型参考传入。
// 如果AI路由未启用或调用失败，返回 nil 和 error，调用方应回退到默认路由。
func AnalyzeMessageRoute(messageContent string, ctx RouteContext) ([]int64, error) {
	if !config.GetAIRoutingEnabled() {
		return nil, fmt.Errorf("AI路由未启用")
	}

	if len(ctx.AvailableGroups) == 0 {
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
			{Role: "system", Content: buildRoutingPrompt(ctx)},
			{Role: "user", Content: messageContent},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("AI路由: 序列化请求失败: %v", err)
		return nil, err
	}

	logger.Info("AI路由: 正在调用 %s 分析消息 (类型=%s, 来源群=%d, 长度=%d字符, 可用群组=%d个)",
		apiURL, ctx.MsgType, ctx.SourceGroupID, len(messageContent), len(ctx.AvailableGroups))

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("AI路由: 创建请求失败: %v", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := AIHTTPClient.Do(req)
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

	groupIDs, err := parseAIRoutingResponse(aiContent, ctx.AvailableGroups)
	if err != nil {
		logger.Error("AI路由: 解析路由判定失败: %v", err)
		return nil, err
	}

	logger.Info("AI路由: 判定目标群组: %v", groupIDs)
	return groupIDs, nil
}

// buildRoutingPrompt 构建AI路由的系统提示词。
// 核心设计：将所有群聊的摘要作为上下文提供给AI，让它基于内容相关性和话题匹配进行路由判定。
func buildRoutingPrompt(ctx RouteContext) string {
	allSummaries := cache.GetAllSummaries()

	var sb strings.Builder
	sb.WriteString("你是一个QQ群消息路由助手。你需要判断AI的回复消息应该发送到哪个或哪些群聊。\n\n")

	// 列出所有可用群聊及其摘要
	sb.WriteString("## 可用群聊及近期对话摘要\n")
	for _, gid := range ctx.AvailableGroups {
		marker := ""
		if gid == ctx.SourceGroupID {
			marker = " ← 消息来源群（用户在此群发起对话）"
		}
		sb.WriteString(fmt.Sprintf("\n### 群ID: %d%s\n", gid, marker))

		summaries, hasSummaries := allSummaries[gid]
		if hasSummaries && len(summaries) > 0 {
			sb.WriteString("近期对话摘要：\n")
			for i, s := range summaries {
				sb.WriteString(fmt.Sprintf("%d. [关键词:%s] %s\n", i+1, s.Keyword, s.Content))
			}
		} else {
			sb.WriteString("(暂无近期对话记录)\n")
		}
	}

	sb.WriteString("\n## 路由规则\n")
	sb.WriteString("1. **默认行为**：AI回复应仅发送到消息来源群（即发起对话的那个群），除非有明确理由需要扩散\n")
	sb.WriteString("2. **内容匹配**：如果回复内容与某个非来源群的近期对话话题高度相关，可额外发送到该群\n")
	sb.WriteString("3. **跨群扩散**：仅当消息内容明显涉及所有群聊都应知晓的信息时（如全局公告、紧急通知），才发送到多个群\n")
	sb.WriteString("4. **保守原则**：拿不准时，宁可少发不要多发。错误扩散比遗漏更糟糕\n")
	sb.WriteString("5. 返回格式：{\"group_ids\": [群ID数组]}\n")
	sb.WriteString("6. 只返回JSON，不要包含任何其他文字或解释\n")

	return sb.String()
}

// parseAIRoutingResponse 从AI的文本响应中提取群组ID列表
func parseAIRoutingResponse(aiContent string, availableGroups []int64) ([]int64, error) {
	var decision types.AIRoutingDecision
	if err := json.Unmarshal([]byte(aiContent), &decision); err == nil {
		return validateGroupIDs(decision.GroupIDs, availableGroups)
	}

	jsonStart := strings.Index(aiContent, "{")
	jsonEnd := strings.LastIndex(aiContent, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := aiContent[jsonStart : jsonEnd+1]
		if err := json.Unmarshal([]byte(jsonStr), &decision); err == nil {
			return validateGroupIDs(decision.GroupIDs, availableGroups)
		}
	}

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
