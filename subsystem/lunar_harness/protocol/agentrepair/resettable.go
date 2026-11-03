package agentrepair

// resettable.go 「无持久对话上下文」类智能体的统一修复目标。
//
// Go 智能体分两类：
//   - 持有跨轮上下文的（核心人格、互助论坛）：自己实现 trim / drop_images / clear；
//   - 按次执行的（检索、观影、桌面、桥接、创作、文件编辑、视觉、文献解析等）：
//     没有可裁剪的消息历史，修复动作统一为 reset——清掉这一轮的在途状态、
//     最近记录或缓存，让下一次调用从干净状态开始。
//
// 后者统一用 RegisterResettable 登记：读上下文复用 agentdump 的导出，
// 各包只需提供一个幂等的重置函数，不必重复实现 Inspect。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// actionReset 按次执行类智能体唯一支持的修复动作。
const actionReset = "reset"

// RegisterResettable 登记一个「只能重置」的修复目标。
// id 需与该智能体的 agentdump 导出标识一致，便于拯救者先读后改。
func RegisterResettable(id, label, description string, reset func() string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	Register(Handler{
		ID:          id,
		Label:       label,
		Description: description,
		Actions:     []string{actionReset},
		Inspect:     func(limit int) (any, error) { return inspectDump(id) },
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

// inspectDump 复用数据导出结果作为上下文快照；导出不是 JSON 时按文本返回。
func inspectDump(id string) (any, error) {
	text, err := agentdump.Export(id, false)
	if err != nil {
		return nil, err
	}
	var value any
	if json.Unmarshal([]byte(text), &value) == nil {
		return value, nil
	}
	return text, nil
}
