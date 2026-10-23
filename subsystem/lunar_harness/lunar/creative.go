package lunar

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// runCreativeLoop 创作型子智能体的通用推理循环
func runCreativeLoop(
	roleName string,
	builder *ModelBuilder,
	taskContent any,
	tools []ToolCall,
	maxIterations int,
	execute func(ToolCallItem) string,
	collect func(ToolCallItem),
	summary func() string,
) string {
	builder.writeContext(PostMessage{Role: RoleUser, Content: taskContent})
	rejectionReason := ""
	for i := 0; i < maxIterations; i++ {
		logger.Info(lunarCoreModule, "[%s] 第 %d 轮推理", roleName, i+1)
		response, err := builder.run(nil, tools)
		if err != nil {
			logger.Warn(lunarCoreModule, "[%s] 第 %d 轮推理失败: %v", roleName, i+1, err)
			break
		}
		if len(response.Body.Choices) == 0 {
			break
		}
		message := response.Body.Choices[0].Message
		if len(message.ToolCalls) == 0 {
			if strings.TrimSpace(message.Content) != "" {
				rejectionReason = message.Content
			}
			builder.writeContext(messageFromAssistant(message))
			break
		}
		builder.writeContext(messageFromAssistant(message))
		for _, call := range message.ToolCalls {
			result := execute(call)
			builder.writeContext(PostMessage{Role: RoleTool, Content: result, ToolCallID: call.ID})
			collect(call)
		}
	}
	result := summary()
	if result == "" {
		if rejectionReason != "" {
			result = rejectionReason
		} else {
			result = "月华认为此次无需进行创作"
		}
	}
	return result
}
