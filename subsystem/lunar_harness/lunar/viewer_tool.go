package lunar

// viewer_tool.go 观影者原生工具：不经过 LTPX，避免破坏 use_the_program 的标准参数语义。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/BilibiliLife"
)

// viewerTools 观影者工具定义（原生 function 工具，flow/share 是它自己的参数）。
var viewerTools = []ToolCall{
	functionTool("viewer", "观影者（B 站）：看视频与评论、写日记动态或仅回复未读消息。flow 选择流程；share 选择是否发分享评论并 @ 用户；instruction 可表达本轮想看的内容。",
		map[string]any{
			"flow": map[string]any{
				"type":        "string",
				"enum":        []string{"standard", "watch_comment", "reply_only"},
				"description": "standard=回复未读消息+看视频与评论+写日记动态；watch_comment=看视频与评论+写日记动态；reply_only=仅回复未读消息",
			},
			"share": map[string]any{
				"type":        "boolean",
				"default":     true,
				"description": "是否从本轮看过的视频中选一条，发分享评论并 @ 用户；省略即视为 true",
			},
			"instruction": map[string]any{
				"type":        "string",
				"description": "本轮想看的内容或想法（可选）。会据此提炼本轮选片偏好，仅本次调用生效",
			},
		}, []string{"flow"}),
}

// handleViewer 执行观影者流程，返回日记/结果文本。
func handleViewer(args map[string]any) (result ToolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ToolResult{Text: fmt.Sprintf("观影者执行异常: %v", recovered)}
		}
	}()

	flow, ok := BilibiliLife.ParseFlow(stringArg(args, "flow"))
	if !ok {
		return ToolResult{Text: "flow 只能取 standard / watch_comment / reply_only"}
	}
	share := true
	if raw, exists := args["share"]; exists {
		switch value := raw.(type) {
		case bool:
			share = value
		case string:
			share = strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}

	instruction := strings.TrimSpace(stringArg(args, "instruction"))
	text, err := BilibiliLife.RunFlow(flow, share, instruction)
	if err != nil {
		if strings.TrimSpace(text) != "" {
			return ToolResult{Text: "【观影者执行失败】" + err.Error() + "\n已完成部分：\n" + text}
		}
		return ToolResult{Text: "【观影者执行失败】" + err.Error()}
	}
	if strings.TrimSpace(text) == "" {
		text = "观影者已完成本次流程"
	}
	return ToolResult{Text: text}
}
