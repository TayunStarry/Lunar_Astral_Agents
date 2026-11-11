package WishManager

// type.go 愿望管理智能体的类型定义。

import (
	"encoding/json"
	"fmt"

	"LunarSubsystem/LunarHarness/engine/wishchain"
)

// Result 一次愿望管理操作的结构化结果。
type Result struct {
	// OK 操作是否成功
	OK bool `json:"ok"`
	// Action 实际操作（query / add / update / complete / delete / priority）
	Action string `json:"action,omitempty"`
	// Item 本次新增 / 改写 / 完成 / 删除的愿望
	Item *wishchain.Wish `json:"item,omitempty"`
	// Handles 资源句柄（愿望不涉及资源句柄，固定为空数组）
	Handles []string `json:"handles"`
	// Summary 操作简述
	Summary string `json:"summary"`
	// NeedConfirm 是否因容量上限需要向对话者确认保留哪些愿望
	NeedConfirm bool `json:"need_confirm,omitempty"`
	// Detail 附加细节
	Detail map[string]any `json:"detail,omitempty"`
}

// JSON 序列化为回填给模型的 JSON 字符串。
func (r Result) JSON() string {
	raw, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false,"handles":[],"summary":"结果序列化失败"}`
	}
	return string(raw)
}

// wishTexts 把当前待实现愿望转成可读列表，供容量确认使用。
func wishTexts(items []wishchain.Wish) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprintf("P%d｜%s｜ID:%s", item.Priority, item.Content, item.ID))
	}
	return out
}
