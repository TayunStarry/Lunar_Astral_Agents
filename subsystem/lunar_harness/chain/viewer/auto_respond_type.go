package viewer

// auto_respond_type.go B 站自动响应的类型定义与依赖端口。
//
// 管线只做调度：浏览器、回应智能体、发布能力与重试队列全部通过接口注入，
// 生产由 defaultAutoRespondDeps 绑定真实实现，测试换成假实现即可离线跑。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/activity"
)

const (
	// sourceAt / sourceVideo / sourceDynamic 三类回应来源的统计标识。
	sourceAt      = "at"
	sourceVideo   = "video"
	sourceDynamic = "dynamic"

	// autoRespondMaxAttempts 单条回应失败后的最多尝试次数（含首次）。
	// 达到上限后移出待回应队列并记录原因，不再无限重试，也不阻塞同批其它条目。
	autoRespondMaxAttempts = 3
	// autoRespondErrorRunes 失败原因写入队列时的截断长度。
	autoRespondErrorRunes = 200
)

// SourceReplyStat 单一来源的处理统计。
type SourceReplyStat struct {
	// Found 本轮取到的待回应条目数（含上轮失败留下的重试条目）。
	Found int `json:"found"`
	// Replied 成功发布回应的条目数。
	Replied int `json:"replied"`
	// Retrying 本轮失败、已保留到下一轮继续尝试的条目数。
	Retrying int `json:"retrying"`
	// Dropped 连续失败达到上限、已记录原因并放弃的条目数。
	Dropped int `json:"dropped"`
}

// AutoRespondResult 一次自动响应的对账结果。
type AutoRespondResult struct {
	// Triggered 三项触发条件是否全部满足；false 表示本轮没有执行任何动作。
	Triggered bool   `json:"triggered"`
	Reason    string `json:"reason,omitempty"`

	At      SourceReplyStat `json:"at"`
	Video   SourceReplyStat `json:"video"`
	Dynamic SourceReplyStat `json:"dynamic"`
}

// Replied 三类来源合计成功条数。
func (r AutoRespondResult) Replied() int {
	return r.At.Replied + r.Video.Replied + r.Dynamic.Replied
}

// Failed 三类来源合计失败条数（含已放弃）。
func (r AutoRespondResult) Failed() int {
	return r.At.Retrying + r.Video.Retrying + r.Dynamic.Retrying +
		r.At.Dropped + r.Video.Dropped + r.Dynamic.Dropped
}

// statOf 按来源取统计槽；未知来源返回 nil。
func (r *AutoRespondResult) statOf(source string) *SourceReplyStat {
	switch source {
	case sourceAt:
		return &r.At
	case sourceVideo:
		return &r.Video
	case sourceDynamic:
		return &r.Dynamic
	}
	return nil
}

// replySession 发布一条回应所需的最小能力（*webSession 实现）。
//
// 单独抽出来是为了让观影 Tick 的被动回应与生活链的自动响应共用同一份发布逻辑。
type replySession interface {
	replyByAPI(item inboxItem, message string) error
}

// autoRespondSession 自动响应需要的浏览器能力（*webSession 实现）。
type autoRespondSession interface {
	replySession
	ensureLogin() error
	fetchInboxAllItems() ([]inboxItem, error)
	close()
}

// autoRespondDeps 自动响应的全部外部依赖。
type autoRespondDeps struct {
	open        func() (autoRespondSession, error)
	filter      func([]inboxItem) []inboxItem
	generate    func(inboxItem) (string, error)
	reply       func(autoRespondSession, inboxItem, string) error
	markSeen    func(inboxItem)
	loadQueue   func() (respondQueue, error)
	saveQueue   func(respondQueue) error
	maxAttempts int
	setActivity func(string)
}

// defaultAutoRespondDeps 绑定生产实现：浏览器、回应智能体、发布与重试队列。
func defaultAutoRespondDeps() autoRespondDeps {
	return autoRespondDeps{
		open: func() (autoRespondSession, error) { return openBilibiliBrowser() },
		filter: func(items []inboxItem) []inboxItem {
			return filterUnreadInbox(items, loadInboxSeen(), time.Now())
		},
		generate: generateInboxReply,
		reply: func(session autoRespondSession, item inboxItem, text string) error {
			return publishReplyWith(session, item, text)
		},
		markSeen:    func(item inboxItem) { markInboxSeen(item) },
		loadQueue:   loadRespondQueue,
		saveQueue:   saveRespondQueue,
		maxAttempts: autoRespondMaxAttempts,
		setActivity: activity.Set,
	}
}
