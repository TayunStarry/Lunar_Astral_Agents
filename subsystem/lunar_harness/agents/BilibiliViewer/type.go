package BilibiliViewer

// type.go 哔哩观影的共享类型定义。
//
// 核心机制（Tick 驱动）：
//   - 启动阶段一次性完成「关键词 → 采集 → 视频印象 → 排序 → 审核/评价/切片数」；
//   - 第一个 Tick 发布观影动态与评论区，之后每秒 Tick 处理一个视频切片，全部切片完成后合并入库；
//   - 动态与评论只依赖视频印象和审核评价，不依赖观看切片。
//
// 发言场景只有两处：自己的动态评论区、以及被动收到的 @/追评回复；
// 不再主动去别人的视频评论区留言。

import (
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

// MentionTarget 分享时 @ 的目标：来自当前账号的特别关注列表。
type MentionTarget struct {
	Name string `json:"name"`
	UID  int64  `json:"uid,omitempty"`
}

// Config 哔哩观影的长期配置（local_data/bilibili_life.json）。
//
// videos_per_round 已移除：候选数量固定为 4-8 个，由机制决定。
type Config struct {
	// PreferenceKeywords 更倾向看的主题词（标题/标签命中即加分）
	PreferenceKeywords []string `json:"preference_keywords"`
	// DailyLimit 每日可调用哔哩观影刷视频的次数上限
	DailyLimit int `json:"daily_limit"`
	// RepliesPerRound 一轮可回应几次被动消息（@ 与追评共享）
	RepliesPerRound int `json:"replies_per_round"`

	// DesireKeywords 本轮搜索关键词：由对话者指令结合 PreferenceKeywords 与长期记忆生成，
	// 只在本次运行内生效，不写回配置文件。
	DesireKeywords []string `json:"-"`
}

// videoLink 列表页上的视频卡片。
type videoLink struct {
	Href   string `json:"href"`
	Title  string `json:"title"`
	Score  int    `json:"-"`
	Source string `json:"-"`
}

// videoInfo 视频的基础信息（详情接口）。
type videoInfo struct {
	Bvid        string
	Aid         int64
	Cid         int64
	Title       string
	Owner       string
	Duration    int
	URL         string
	Desc        string
	Pic         string
	PublishTime int64
	// 热度指标：参与候选综合匹配度加权
	View     int64
	Like     int64
	Coin     int64
	Favorite int64
	ReplyNum int64
	Danmaku  int64
}

// commentItem 一条评论（视频印象与风险复核共用）。
type commentItem struct {
	User    string `json:"user"`
	Content string `json:"content"`
	Like    int    `json:"like"`
}

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

// videoRecord 本轮一个候选视频的完整采集与处理结果。
type videoRecord struct {
	Link       videoLink
	Info       videoInfo
	Tags       []string        `json:"tags,omitempty"`
	Impression videoImpression `json:"impression,omitempty"`

	Comments []commentItem `json:"comments,omitempty"`
	Danmaku  []string      `json:"danmaku,omitempty"`

	// Score 与本轮观影意图的综合匹配度（嵌入相似度 + 热度加权）
	Score float64 `json:"score,omitempty"`

	// Slices 审核阶段按视频时长决定的期望观看切片数
	Slices int `json:"slices,omitempty"`
	// Evaluation 主模型生成的「视频评价」（发布到动态评论区）
	Evaluation string `json:"evaluation,omitempty"`

	Risky      bool   `json:"risky,omitempty"`
	RiskReason string `json:"risk_reason,omitempty"`

	// 切片观看阶段的临时资源
	LocalPath string                `json:"local_path,omitempty"`
	Segments  []module.MediaSegment `json:"segments,omitempty"`
	Summaries []string              `json:"summaries,omitempty"`
	// WatchSummary 全部切片合并后的整体观看感受
	WatchSummary string `json:"watch_summary,omitempty"`

	CommentText string `json:"comment_text,omitempty"` // 动态评论区里的评价条目
	ShareText   string `json:"share_text,omitempty"`   // @分享评论正文
}

// videoReview 主模型审核 + 评价 + 切片数的一次性结构化输出。
type videoReview struct {
	Pass       bool   `json:"pass"`
	Reason     string `json:"reason"`
	Evaluation string `json:"evaluation"`
	Slices     int    `json:"slices"`
}

// lifeActivity 本轮互动记录：写长期记忆与动态时交给模型。
type lifeActivity struct {
	Kind    string `json:"kind"` // inbox / watch / dynamic / share
	Where   string `json:"where,omitempty"`
	Target  string `json:"target,omitempty"`
	Content string `json:"content,omitempty"`
	Reply   string `json:"reply,omitempty"`
	Note    string `json:"note,omitempty"`
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

// stepKind Tick 状态机的原子步骤。
type stepKind int

const (
	// stepWatch 观看并理解一个视频切片
	stepWatch stepKind = iota
	// stepMerge 合并单个视频的全部切片理解并入库
	stepMerge
	// stepPublish 发布观影动态 + 评价条目 + @分享
	stepPublish
)

// pipelineStep Tick 计划表中的一步。
type pipelineStep struct {
	Kind  stepKind
	Video *videoRecord
	Slice int
}

// memoryEntry 统一记忆库里的一条哔哩观影记忆。
//
// 正文格式：[B站视频][印象/评价/习惯/问题/吐槽/...]<正文>
type memoryEntry struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	Time int64  `json:"time,omitempty"`
}

// dynamicCommentTarget 一条动态真正的评论区（type + oid）。
//
// 动态 ID 不等于评论 oid：纯文字动态经 dynamic_svr 发布后常被归为图文动态
// （comment_type=11），必须用详情接口返回的 comment_id_str/comment_type。
type dynamicCommentTarget struct {
	Type int
	Oid  string
}
