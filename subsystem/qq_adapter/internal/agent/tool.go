package agent

import (
	"QQAdapter/internal/processor" // 核心处理器
	"encoding/json"                // 用于JSON编码和解码
	"fmt"                          // 格式化输出
)

// GetTools 获取所有可用工具
func GetTools(ToolChoice string) []Tool {
	if ToolChoice == "none" {
		return []Tool{}
	}
	return []Tool{
		{
			Type: "function",
			Function: FunctionData{
				Name:        "save_to_knowledge_base",
				Description: "当你需要将一段给保存与记录下来时, 可调用该工具",
				Parameters: ParameterData{
					Type: "object",
					Properties: map[string]PropertyData{
						"content": {
							Type:        "string",
							Description: "经过总结提炼的, 需要保存的内容",
						},
					},
					Required: []string{"content"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionData{
				Name:        "diffusion_generation",
				Description: "根据文本描述生成图像。如需进行图像创作，请调用此函数。",
				Parameters: ParameterData{
					Type: "object",
					Properties: map[string]PropertyData{
						"prompt": {
							Type:        "string",
							Description: "图像生成的正向描述文本",
						},
						"negative_prompt": {
							Type:        "string",
							Description: "负面提示文本，用于排除图像中不希望出现的元素",
						},
						"use_reference": {
							Type:        "boolean",
							Description: "是否使用上一次生成的图像作为参考，默认值为 false",
						},
						"strength": {
							Type:        "number",
							Description: "参考图像的影响强度，取值范围为 0 到 1，默认值为 0.65",
						},
						"cfg_scale": {
							Type:        "number",
							Description: "提示词权重调节参数，取值范围为 0 到 2，默认值为 1.0",
						},
					},
					Required: []string{"prompt"},
				},
			},
		},
	}
}

// ExecuteTool 执行工具调用
func ExecuteTool(toolCall processor.ToolCall, processor *processor.Handle) (string, error) {
	switch toolCall.Function.Name {
	case "save_to_knowledge_base":
		// 解析参数
		var args RequestContent
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("解析工具参数失败: %v", err)
		}
		return processor.SaveToKnowledgeBase(args.Content, "knowledge/lunar_notes.json")
	case "diffusion_generation":
		// 解析参数
		var args RequestGeneration
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("解析工具参数失败: %v", err)
		}
		// 验证必需参数
		if args.Prompt == "" {
			return "", fmt.Errorf("生成图片需要提供正向提示文本")
		}
		// 设置默认值
		strength := 0.65
		cfgScale := 1.0
		// 判断是否提供了强度参数
		if args.Strength != nil {
			strength = *args.Strength
		}
		// 判断是否提供了CFG缩放参数
		if args.CfgScale != nil {
			cfgScale = *args.CfgScale
		}
		// 调用图片生成服务
		return processor.GenerateImage(args.Prompt, args.NegativePrompt, args.UseReference, strength, cfgScale)
	default:
		return "", fmt.Errorf("未知的工具: %s", toolCall.Function.Name)
	}
}
