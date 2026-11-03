package Dialogue

// agent_scope.go 智能体聚合工具的公共输出与「职能范围描述」。
//
// details=true 时只读取这里的描述文案，不调用任何子智能体，也不产生副作用。

import (
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// schemaTool 用完整参数 schema 构造函数工具定义（聚合工具统一走这里）。
func schemaTool(name, description string, parameters map[string]any) ToolCall {
	return ToolCall{Type: "function", Function: ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  parameters,
	}}
}

// toolJSON 把统一输出原语序列化为工具返回文本。
func toolJSON(result agentdispatch.Result) ToolResult {
	return ToolResult{Text: result.JSON()}
}

// techniqueScope 返回技法创作各分支的职能范围描述。
func techniqueScope(kind string) (string, bool) {
	switch kind {
	case techniquePainter:
		return "绘制者子智能体：按自然语言创作图像，支持文生图、图生图、立绘、合照与超分；handle 可传参考图句柄（如 @I.003）。", true
	case techniqueMusician:
		return "演奏者子智能体：创作音乐作品，输出旋律、和弦与乐器编排说明。", true
	case techniqueScreenshoter:
		return "截图者子智能体：截取屏幕、窗口或指定区域，并把截图展示给用户。", true
	case techniqueFileOperations, techniqueCreatorLegacy, techniqueFileEditorLegacy:
		return "文件操作子智能体：统一编排文件读写、文档整理、压缩包处理、文献读取与脚本运行；按目标自动路由到文件编辑、归档解析 / 操作与文献解析，写入自动快照、审查与回滚。", true
	}
	return "", false
}

// networkScope 返回联网探索各分支的职能范围描述。
func networkScope(kind string) (string, bool) {
	switch kind {
	case networkWebSearchTool:
		return "网页检索子智能体：联网搜索并用自然语言返回摘要与来源。", true
	case networkBilibiliTool:
		return "哔哩观影子智能体：登记观影事件，由生活链进入 B 站观影流程。", true
	case networkMutualForumTool:
		return "互助论坛子智能体：登记论坛事件，由生活链进入互助论坛流程。", true
	case networkQQSendTool:
		return "通讯桥接子智能体：向指定 QQ 群或好友发送文本、图片或文件；handle 传待发送的图片 / 文件句柄。", true
	}
	return "", false
}
