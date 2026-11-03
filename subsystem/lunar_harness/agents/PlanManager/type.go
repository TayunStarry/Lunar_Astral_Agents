package PlanManager

// type.go 计划管理智能体的类型定义。

import (
	"encoding/json"
	"fmt"

	"LunarSubsystem/LunarHarness/engine/plans"
)

// Result 一次计划管理操作的结构化结果。
type Result struct {
	// OK 操作是否成功
	OK bool `json:"ok"`
	// Action 实际操作（query / create / update / delete）
	Action string `json:"action,omitempty"`
	// Item 本次新增 / 改写 / 删除的计划项
	Item *plans.Item `json:"item,omitempty"`
	// Handles 资源句柄（计划项不涉及资源句柄，固定为空数组）
	Handles []string `json:"handles"`
	// Summary 操作简述
	Summary string `json:"summary"`
	// Detail 附加细节
	Detail map[string]any `json:"detail,omitempty"`
}

// JSON 序列化为回填给模型的 JSON 字符串。
func (r Result) JSON() string {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"handles":[],"summary":"结果序列化失败"}`)
	}
	return string(raw)
}
