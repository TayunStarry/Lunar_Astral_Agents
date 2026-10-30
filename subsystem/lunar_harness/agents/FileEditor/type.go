package FileEditor

// type.go 文件编辑智能体的公共类型定义。

import (
	"encoding/json"
	"fmt"
)

// ==== 结构化操作结果 ====
//
// 所有工具返回值统一序列化为 JSON 字符串，固定包含：
//   - handles：本次改动涉及的资源句柄（无句柄时为空数组，不省略）；
//   - summary：本次操作简述（供调用方直接转述给用户）。
type OpResult struct {
	OK      bool           `json:"ok"`
	Op      string         `json:"op"`
	Path    string         `json:"path,omitempty"`
	Handles []string       `json:"handles"`
	Summary string         `json:"summary"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// JSON 序列化为回填给模型/调用方的 JSON 字符串。
func (r OpResult) JSON() string {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"op":%q,"handles":[],"summary":"结果序列化失败"}`, r.Op)
	}
	return string(raw)
}

// newResult 构造成功结果（handles 保证为非 nil 数组）。
func newResult(op, path string, summary string, handles []string, detail map[string]any) OpResult {
	if handles == nil {
		handles = []string{}
	}
	return OpResult{OK: true, Op: op, Path: path, Handles: handles, Summary: summary, Detail: detail}
}

// failResult 构造失败结果。
func failResult(op, path string, err error) OpResult {
	message := op + " 失败：" + err.Error()
	return OpResult{
		OK:      false,
		Op:      op,
		Path:    path,
		Handles: []string{},
		Summary: previewRunes(message, 200),
		Detail:  map[string]any{"error": err.Error()},
	}
}

// fileEntry 目录条目。
type fileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

// searchHit 内容检索命中。
type searchHit struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Text string `json:"text"`
}

// ==== 模型调用类型（OpenAI 兼容 function calling） ====

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []toolCallItem `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type toolCallItem struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type modelResponse struct {
	Content   string
	Reasoning string
	ToolCalls []toolCallItem
}

type chatCompletion struct {
	Choices []struct {
		Message struct {
			Content          string         `json:"content"`
			ReasoningContent string         `json:"reasoning_content"`
			ToolCalls        []toolCallItem `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}
