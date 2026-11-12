package ViewerRemark

// type.go 观影者-观感的输入输出类型。

import "encoding/json"

// RemarkInput 评论生成的输入：视频印象 + 特别关注候选 + 对话者的附加要求。
type RemarkInput struct {
	// Material 视频印象（标题、评论、弹幕、时长、标签、简介、UP 主与画面理解）
	Material string
	// Candidates 可 @ 的特别关注候选（只含展示名与序号，不含内部 ID）
	Candidates []MentionCandidate
	// Model 模型名
	Model string
}

// MentionCandidate 可 @ 的特别关注候选：序号用于让模型稳定指代，展示名用于自然表达。
type MentionCandidate struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// Remark 一次评论生成的结构化结果。
type Remark struct {
	// Comments 1-2 条可直接发布的评论，每条各自带自己的 @ 决策。
	Comments []RemarkComment `json:"comments"`
}

// RemarkComment 一条评论与它自己的 @ 决策。
type RemarkComment struct {
	// Text 可直接发布的评论正文
	Text string `json:"text"`
	// Mention 这条评论想 @ 的特别关注人数：0 表示不 @，1-3 表示想 @ 几个人
	Mention int `json:"mention"`
	// At 模型指定的候选（展示名或候选序号，原样保留，由链按候选清单校验）
	At []string `json:"at,omitempty"`
}

// remarkToolArgs submit_remark 的参数结构。
type remarkToolArgs struct {
	Comments []remarkToolComment `json:"comments"`
}

// remarkToolComment 提交工具里的一条评论。
type remarkToolComment struct {
	Text    string            `json:"text"`
	Mention *int              `json:"mention"`
	At      []json.RawMessage `json:"at"`
}
