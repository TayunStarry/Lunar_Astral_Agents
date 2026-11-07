package agentrepair

// readonly.go 「按次执行」类智能体的统一只读登记入口。
//
// 许多 Go 智能体没有跨轮对话上下文，但仍需要让维修侧看到它们最近一次运行的状态。
// 它们用 RegisterReadonly 登记：显式传入与 agentdump 同一份导出函数作为 inspect，
// 保证诊断目标与导出目标共用单一数据源；省略 inspect 时退回按 ID 查 agentdump。
//
// 这里只登记读取能力，不登记任何修复动作。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// RegisterReadonly 登记一个只读目标。
//
// inspect 通常直接复用该包 agentdump 导出的同一个 Export 函数
// （如 func(int) (any, error) { return exportContext(false) }），
// 这样诊断目标与导出目标不会因 ID 拼写不一致而出现双源真相。
func RegisterReadonly(id, label, description string, inspect ...func(limit int) (any, error)) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	inspectFn := func(limit int) (any, error) { return inspectDump(id) }
	if len(inspect) > 0 && inspect[0] != nil {
		inspectFn = inspect[0]
	}
	Register(Handler{
		ID:          id,
		Label:       label,
		Description: description,
		Inspect:     inspectFn,
	})
}

// inspectDump 兼容路径：按 ID 复用数据导出结果作为上下文快照；
// 导出不是 JSON 时按文本返回。ID 未注册时给出可定位的明确错误。
func inspectDump(id string) (any, error) {
	text, err := agentdump.Export(id, false)
	if err != nil {
		return nil, fmt.Errorf("诊断目标 %s 未显式提供 inspect，且未注册同名上下文导出: %w", id, err)
	}
	var value any
	if json.Unmarshal([]byte(text), &value) == nil {
		return value, nil
	}
	return text, nil
}
