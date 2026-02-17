package openai

import (
	"encoding/json"
	"fmt"
	"transponder/internal/message"
)

// Tool 工具结构体
type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef 函数定义结构体
type FunctionDef struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Parameters  ParameterDef `json:"parameters"`
}

// ParameterDef 参数定义结构体
type ParameterDef struct {
	Type       string                 `json:"type"`
	Properties map[string]PropertyDef `json:"properties"`
	Required   []string               `json:"required"`
}

// PropertyDef 属性定义结构体
type PropertyDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolCall 工具调用结构体
type ToolCall struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolCallResponse 工具调用响应结构体
type ToolCallResponse struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolResponse 工具响应结构体
type ToolResponse struct {
	Role       string `json:"role"`
	Content    string `json:"content,omitempty"`
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
}

// GetTools 获取所有可用工具
func GetTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "save_to_knowledge_base",
				Description: "当你觉得一段内容(文本)有作为笔记或总结的价值时，可调用该工具将其保存到本地知识库中",
				Parameters: ParameterDef{
					Type: "object",
					Properties: map[string]PropertyDef{
						"content": {
							Type:        "string",
							Description: "要保存到知识库的文本内容",
						},
					},
					Required: []string{"content"},
				},
			},
		},
	}
}

// ExecuteTool 执行工具调用
func ExecuteTool(toolCall ToolCall, processor *message.Processor) (string, error) {
	switch toolCall.Function.Name {
	case "save_to_knowledge_base":
		// 解析参数
		var args struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("解析工具参数失败: %v", err)
		}
		return processor.SaveToKnowledgeBase(args.Content, "knowledge/lunar_notes.json")
	default:
		return "", fmt.Errorf("未知的工具: %s", toolCall.Function.Name)
	}
}
