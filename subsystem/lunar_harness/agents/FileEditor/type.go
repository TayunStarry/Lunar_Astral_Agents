package FileEditor

// type.go 文件编辑智能体的公共类型定义。

import (
	"encoding/json"
	"fmt"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// ==== 结构化操作结果 ====
//
// 所有工具返回值统一序列化为 JSON 字符串，固定包含：
//   - handles：本次改动涉及的资源句柄（无句柄时为空数组，不省略）；
//   - summary：本次操作简述（供调用方直接转述给用户）。
//
// 智能体聚合工具（task_scheduling）额外携带 Dispatch：序列化时只输出
// {result, handle, error}，不混入本智能体的普通结果字段。
type OpResult struct {
	OK       bool                  `json:"ok"`
	Op       string                `json:"op"`
	Path     string                `json:"path,omitempty"`
	Handles  []string              `json:"handles"`
	Summary  string                `json:"summary"`
	Detail   map[string]any        `json:"detail,omitempty"`
	Dispatch *agentdispatch.Result `json:"-"`
}

// JSON 序列化为回填给模型/调用方的 JSON 字符串。
func (r OpResult) JSON() string {
	if r.Dispatch != nil {
		return r.Dispatch.JSON()
	}
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

// ==== 模型调用类型（统一复用基座类型） ====

// toolDefinition 发给模型的函数工具定义。
type toolDefinition = agentbase.ToolDefinition

// toolFunction 函数描述与参数 Schema。
type toolFunction = agentbase.ToolFunctionDef
