package agentrepair

// resettable.go 「无持久对话上下文」类智能体的统一修复目标。
//
// Go 智能体分两类：
//   - 持有跨轮上下文的（核心人格、互助论坛）：自己实现 trim / drop_images / clear；
//   - 按次执行的（检索、观影、桌面、桥接、创作、文件编辑、视觉、文献解析等）：
//     没有可裁剪的消息历史，修复动作统一为 reset——清掉这一轮的在途状态、
//     最近记录或缓存，让下一次调用从干净状态开始。
//
// 后者统一用 RegisterResettable 登记：调用方显式传入与 agentdump 同一份导出函数
// 作为 inspect，修复目标与导出目标共用单一数据源；仅未迁移的旧调用方才退回按
// ID 隐式查 agentdump（未注册时明确降级为可列表、Inspect 报错）。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// actionReset 按次执行类智能体唯一支持的修复动作。
const actionReset = "reset"

// RegisterResettable 登记一个「只能重置」的修复目标。
//
// inspect 为显式的上下文快照读取函数，通常直接复用该包 agentdump 导出的同一个
// Export 函数（如 func(int) (any, error) { return exportSnapshot(false) }），
// 这样修复目标与导出目标不会因 ID 拼写不一致而出现双源真相。
//
// inspect 可省略以兼容尚未迁移的旧调用方；省略时退回按 id 查 agentdump，
// 若该 ID 未注册上下文导出，目标仍会出现在修复清单（明确降级），
// 但 Inspect 返回带原因的错误，不会静默成功。
func RegisterResettable(id, label, description string, reset func() string, inspect ...func(limit int) (any, error)) {
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
		Actions:     []string{actionReset},
		Inspect:     inspectFn,
		Repair: func(action string, _ map[string]any) (string, error) {
			if action != actionReset {
				return "", fmt.Errorf("%s只支持 reset 动作", label)
			}
			if reset == nil {
				return "", fmt.Errorf("%s没有可重置的状态", label)
			}
			return reset(), nil
		},
	})
}

// inspectDump 兼容路径：按 ID 复用数据导出结果作为上下文快照；
// 导出不是 JSON 时按文本返回。ID 未注册时给出可定位的明确错误。
func inspectDump(id string) (any, error) {
	text, err := agentdump.Export(id, false)
	if err != nil {
		return nil, fmt.Errorf("修复目标 %s 未显式提供 inspect，且未注册同名上下文导出: %w", id, err)
	}
	var value any
	if json.Unmarshal([]byte(text), &value) == nil {
		return value, nil
	}
	return text, nil
}
