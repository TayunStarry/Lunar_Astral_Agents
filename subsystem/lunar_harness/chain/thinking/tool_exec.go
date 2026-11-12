package thinking

import (
	"context"
	"errors"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// executeToolCall 执行单个工具调用并回填上下文（基座 Execute 回调）。
func (role *DialogueRole) executeToolCall(call agentbase.ToolCall) (string, bool, error) {
	name := call.Function.Name
	handler := lookupTool(name)
	if handler == nil {
		content := "未找到工具包: " + name
		role.Messages = append(role.Messages, PostMessage{Role: RoleTool, Content: content, ToolCallID: call.ID})
		reportDialogueFailure(fmt.Errorf("工具执行失败（%s）：未找到工具包", name))
		return content, false, nil
	}
	args, err := agentbase.ParseToolArgs(call.Function.Arguments)
	if err != nil {
		args = map[string]any{}
	}
	// details=true 只是查询复合工具的职能描述，不执行长任务，不推快速应答。
	if details, _ := args["details"].(bool); !details {
		maybeSendFastAck(name)
	}
	result, err := runToolHandler(handler, args)
	if err != nil {
		return "", false, fmt.Errorf("工具执行失败（%s）: %w", name, err)
	}
	role.Messages = append(role.Messages, PostMessage{Role: RoleTool, Content: result.Text, ToolCallID: call.ID})
	if image := normalizeToolImage(result.Image); image != "" {
		content := make([]ContentItem, 0, 2)
		if result.Text != "" {
			content = append(content, ContentItem{Type: ContentText, Text: result.Text})
		}
		// 工具产物（截图、渲染图等）都是静图：这里显式声明，不靠后缀或字节推断
		payload := &URLPayload{Type: mediaobject.TypeImageStatic, URL: image}
		if converted := convertImageResource(payload); converted.Type != "" {
			content = append(content, converted)
		}
		role.Messages = append(role.Messages, PostMessage{Role: RoleUser, Content: content})
		logger.Info(lunarCoreModule, "[工具调用] %s 返回图片数据，长度=%d 字节", name, len(result.Image))
	}
	return result.Text, false, nil
}

// runToolHandler 执行工具处理函数，把 panic 转成可上报的错误（原实现会让整轮直接中断）。
func runToolHandler(handler ToolHandler, args map[string]any) (result ToolResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("工具执行异常: %v", recovered)
		}
	}()
	return handler(args), nil
}

// hasExecutableTool 判断本批调用里是否至少有一个已注册的工具。
func hasExecutableTool(calls []ToolCallItem) bool {
	for _, call := range calls {
		if lookupTool(call.Function.Name) != nil {
			return true
		}
	}
	return false
}

// appendMissingToolResults 回填「未找到工具包」结果，并把缺失工具上报给智能修复。
func (role *DialogueRole) appendMissingToolResults(calls []ToolCallItem) {
	for _, call := range calls {
		content := "未找到工具包: " + call.Function.Name
		role.Messages = append(role.Messages, PostMessage{Role: RoleTool, Content: content, ToolCallID: call.ID})
		reportDialogueFailure(fmt.Errorf("工具执行失败（%s）：未找到工具包", call.Function.Name))
	}
}

// rejectToolCalls 回填未执行工具结果（轮次上限分支）。
func (role *DialogueRole) rejectToolCalls(calls []ToolCallItem) {
	reason := fmt.Sprintf("本轮工具调用已达上限（%d 次），该调用未执行", maxToolRounds)
	for _, call := range calls {
		role.Messages = append(role.Messages, PostMessage{Role: RoleTool, Content: reason, ToolCallID: call.ID})
	}
}

// reportDialogueFailure 把对话者失败上报给智能修复（故障归属 ID 为 dialogue）。
//
// 上报是纯内存登记，不阻塞应答主流程；同时兜住修复钩子自身的 panic，
// 避免上报失败反过来拖垮对话。
func reportDialogueFailure(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Warn(lunarCoreModule, "["+Dialogue.Role+"] 故障上报异常: %v", recovered)
		}
	}()
	Dialogue.ReportFailure(err)
}
