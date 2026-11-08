package SQLWriter

// type.go 语句生成智能体的全部类型定义集中于此。

// chatMessage 语句生成请求的消息项（只含角色与文本）。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
