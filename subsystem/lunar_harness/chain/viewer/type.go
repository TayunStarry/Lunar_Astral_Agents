package viewer

// type.go 哔哩观影的共享类型定义。
//
// 观影流程由生活链的五步回合编排驱动（见 chain/life）：
// 待看列表补齐 → 选片 → 切片理解 → 里程碑 → 被动回应。
// 待看列表与记忆的持久化由 engine/viewerstore 负责，
// B 站接口协议与响应字段映射由 engine/bilibili 负责。

import "LunarSubsystem/LunarHarness/engine/bilibili"

// MentionTarget 分享时 @ 的目标：来自当前账号的特别关注列表。
type MentionTarget = bilibili.MentionTarget

// Config 哔哩观影的长期配置（local_data/config/bilibili.json）。
//
// videos_per_round 已移除：候选数量由机制决定。
type Config struct {
	// PreferenceKeywords 更倾向看的主题词（标题/标签命中即加分），来自配置文件。
	PreferenceKeywords []string `json:"preference_keywords"`

	// DesireKeywords 本轮强倾向词：由观看历史回流的主题词生成，权重高于静态偏好；
	// 只在本次运行内生效，不写回配置文件。
	DesireKeywords []string `json:"-"`

	// WatchlistBudgetSec 待看列表补齐整阶段的时间上限（秒）；<=0 用机制默认值。
	WatchlistBudgetSec int `json:"watchlist_budget_sec,omitempty"`
	// WatchlistCandidateBudgetSec 单个候选（采集+审核+下载）的子预算（秒）；<=0 用机制默认值。
	WatchlistCandidateBudgetSec int `json:"watchlist_candidate_budget_sec,omitempty"`
}

// videoLink 列表页上的视频卡片。
type videoLink struct {
	Href   string `json:"href"`
	Title  string `json:"title"`
	Score  int    `json:"-"`
	Source string `json:"-"`
}

// videoInfo 视频的基础信息（详情接口）。
type videoInfo = bilibili.Video

// commentItem 一条评论（视频印象与风险复核共用）。
type commentItem = bilibili.Comment

// videoImpression 结构化「视频印象」：由轻量模型依据视频可采集信息整理。
type videoImpression struct {
	// Summary 侧面对视频内容的整体理解（一两句话）
	Summary string `json:"summary"`
	// Topics 视频主题/关键词，供嵌入匹配与动态写作使用
	Topics []string `json:"topics"`
	// Tone 观众情绪/氛围（如「轻松吐槽」「硬核讲解」）
	Tone string `json:"tone"`
	// Risky 轻量模型认为有合规风险时置 true，交给主模型复核
	Risky      bool   `json:"risky,omitempty"`
	RiskReason string `json:"risk_reason,omitempty"`
}

// videoRecord 一个候选视频的采集结果（记忆注入与审核共用）。
type videoRecord struct {
	Info       videoInfo
	Tags       []string        `json:"tags,omitempty"`
	Impression videoImpression `json:"impression,omitempty"`

	Comments []commentItem `json:"comments,omitempty"`
	Danmaku  []string      `json:"danmaku,omitempty"`
}

// videoReview 主模型审核 + 评价 + 切片数的一次性结构化输出。
type videoReview struct {
	Pass       bool   `json:"pass"`
	Reason     string `json:"reason"`
	Evaluation string `json:"evaluation"`
	Slices     int    `json:"slices"`
}

// notifyCategory 被动消息的分类（只处理 @ 与追评两类）。
type notifyCategory int

const (
	notifyAt notifyCategory = iota
	notifyVideoReply
	notifyVideoComment
	notifyDynamic
)

// String 类别名称（日志与提示词使用）。
func (c notifyCategory) String() string {
	switch c {
	case notifyAt:
		return "@我的"
	case notifyVideoReply:
		return "视频评论追评"
	case notifyVideoComment:
		return "视频新增评论"
	case notifyDynamic:
		return "动态互动"
	}
	return "通知"
}

// inboxItem 归一化后的待回应消息。
//
// 回应优先通过页面操作发布（找到对方评论 → 点回复 → 输入发送）；
// 页面定位失败时用 RootID/TargetID/SourceID 拼出正确的 root/parent 走接口兜底。
// 三个 ID 都来自消息中心，不需要自己推断评论树。
type inboxItem struct {
	Kind      string         `json:"kind"` // reply / at
	Category  notifyCategory `json:"category"`
	Venue     string         `json:"venue"` // video / dynamic / article
	Type      int            `json:"type"`  // 1=视频，11=图片动态，12=专栏，17=纯文字动态
	Oid       string         `json:"oid"`
	SourceID  string         `json:"source_id,omitempty"`
	RootID    string         `json:"root_id,omitempty"`
	TargetID  string         `json:"target_id,omitempty"`
	MessageID string         `json:"message_id,omitempty"`
	User      string         `json:"user"`
	Mid       int64          `json:"mid,omitempty"`
	Message   string         `json:"message"`
	MyContent string         `json:"my_content,omitempty"`
	Title     string         `json:"title,omitempty"`
	URI       string         `json:"uri,omitempty"`
	Time      int64          `json:"time,omitempty"`
}
