package MutualForum

// auto_reply.go 互助论坛自动响应编排。
//
// 触发条件（必须同时满足）：
//  1. 论坛页面能成功开启（健康探测通过）；
//  2. 存在针对当前月华账号的未读回应消息（服务端事件 + 本地未完成重试队列）。
//
// 任一条件不满足直接返回：页面打不开时不做任何拉取/回复请求；没有未读时只做一次
// 事件拉取就结束，不发起回复请求。
//
// 失败可重试：事件先落入本地重试队列并记录 AckedSeq，才推进服务端游标；只有成功
// 提交的条目才会从队列移除。失败条目留在队列里，下一轮继续尝试；连续失败达到
// 配置上限（forum.json 的 auto_reply_max_retries，默认 3）后移出队列并计入 Dropped，
// 不会阻塞同批其它条目。
//
// 生活链接线方式（本批次不修改 chain/life，由后续批次按下面的方式调用）：
//
//	summary, err := MutualForum.AutoRespondUnread(ctx)
//	if errors.Is(err, MutualForum.ErrForumPageUnavailable) {
//	    // 论坛页面打不开，按不可用处理
//	}
//	// 页面可用但没有待回应：err == nil && summary.Pending == 0
//
// 回复内容由本包的智能体调用模型生成；提交复用既有 CreateComment / Reply（含轮次重试）。

import (
	"context"
	"errors"
	"fmt"

	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// autoRespondDeps 自动响应的可替换依赖；生产实现见 realAutoRespondDeps，
// 测试注入假实现即可验证触发条件、重试与计数，不发起真实网络请求。
type autoRespondDeps struct {
	ensurePage  func(context.Context) error
	pullUnread  func(context.Context, int64) (unreadBatch, error)
	ackUnread   func(context.Context, unreadBatch) error
	store       replyStateStore
	maxRetries  int
	loadContext func(context.Context, unreadReply) (replyContext, error)
	generate    func(context.Context, replyContext) (string, error)
	send        func(context.Context, unreadReply, string) error
	setActivity func(string)
}

// AutoRespondUnread 自动回应论坛里针对当前月华账号的未读回应。
//
// 返回 Summary 区分三种情况：
//   - 页面打不开：返回包装 ErrForumPageUnavailable 的错误；
//   - 页面可用、没有待回应：err == nil，PageOpen=true、Pending=0；
//   - 有待回应：逐条生成并提交，返回成功/失败/放弃计数，并更新活动状态字段。
func AutoRespondUnread(ctx context.Context) (Summary, error) {
	deps, err := realAutoRespondDeps()
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrForumPageUnavailable, err)
	}
	// 不再持有整轮锁：逐条生成在锁外完成，提交时由 CreateComment / Reply 获取
	// 论坛唯一写锁；事件游标推进也只在各自临界区内短暂持锁。
	return autoRespond(ctx, deps)
}

// autoRespond 执行自动响应的纯编排；单条失败只计入 Failed，不阻塞其余条目。
func autoRespond(ctx context.Context, deps autoRespondDeps) (Summary, error) {
	if err := deps.ensurePage(ctx); err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrForumPageUnavailable, err)
	}
	summary := Summary{PageOpen: true}

	state, err := deps.store.Load()
	if err != nil {
		return summary, err
	}

	batch, pullErr := deps.pullUnread(ctx, state.AckedSeq)
	if pullErr != nil {
		if len(state.Items) == 0 {
			return summary, pullErr
		}
		// 拉取失败不应妨碍本地失败条目的重试。
		logger.Warn("LunarMatrix", "拉取论坛未读回应失败，先重试本地待回应条目: %v", pullErr)
	} else {
		state.Items = mergePendingReplies(state.Items, batch.Items)
		if batch.Cursor.ResetRequired && state.AckedSeq != 0 {
			state.AckedSeq = 0
		}
		if batch.Cursor.LastEventSeq > state.AckedSeq {
			state.AckedSeq = batch.Cursor.LastEventSeq
		}
		// 先落盘事件与游标，再推进服务端游标：即使后续提交失败也不会丢事件。
		if saveErr := deps.store.Save(state); saveErr != nil {
			return summary, saveErr
		}
		if ackErr := deps.ackUnread(ctx, batch); ackErr != nil {
			logger.Warn("LunarMatrix", "推进论坛事件游标失败（本地队列已保存，不影响重试）: %v", ackErr)
		}
	}

	summary.Pending = len(state.Items)
	if summary.Pending == 0 {
		return summary, nil
	}

	maxRetries := deps.maxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}
	remaining := make([]pendingReply, 0, len(state.Items))
	for _, entry := range state.Items {
		if ctx.Err() != nil {
			remaining = append(remaining, entry)
			continue
		}
		if err := respondOne(ctx, deps, entry.reply()); err != nil {
			entry.Retries++
			entry.LastError = previewText(err.Error(), maxReplyErrorRunes)
			if entry.Retries >= maxRetries {
				summary.Dropped++
				summary.Failed++
				logger.Warn("LunarMatrix", "互助论坛自动回应放弃事件 seq=%d type=%s：%v", entry.EventSeq, entry.Type, err)
				continue
			}
			summary.Failed++
			remaining = append(remaining, entry)
			continue
		}
		summary.Responded++
	}
	state.Items = remaining
	if saveErr := deps.store.Save(state); saveErr != nil {
		return summary, saveErr
	}

	if text := ReplyActivityText(summary); text != "" {
		deps.setActivity(text)
	}
	return summary, nil
}

// ReplyActivityText 自动回应完成后的活动状态文案；既没有成功也没有失败时返回空串。
// 生活链第 4 步复用它，保证同一件事只有一套措辞。
func ReplyActivityText(summary Summary) string {
	switch {
	case summary.Responded > 0:
		return fmt.Sprintf("在互助论坛回复了 %d 条消息", summary.Responded)
	case summary.Failed > 0:
		return "在互助论坛想回复消息，但没能发出去"
	default:
		return ""
	}
}

// respondOne 加载语境、生成回复并提交。
func respondOne(ctx context.Context, deps autoRespondDeps, item unreadReply) error {
	rc, err := deps.loadContext(ctx, item)
	if err != nil {
		return err
	}
	// 生成回复是一次独立的模型调用：自己起一条实况，避免内容被拼进上一条
	// （对话者/生活链前一步）的实况里。多条待回应 = 同一智能体的多条记录。
	agentbase.BeginLive(roleLabel)
	body, err := deps.generate(ctx, rc)
	agentbase.EndLive(roleLabel)
	if err != nil {
		return err
	}
	return deps.send(ctx, item, body)
}

// realAutoRespondDeps 生产依赖：健康探测、事件拉取、本地重试队列、模型生成与既有提交能力。
func realAutoRespondDeps() (autoRespondDeps, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return autoRespondDeps{}, err
	}
	return autoRespondDeps{
		ensurePage:  ensureForumPageOpen,
		pullUnread:  pullUnreadReplies,
		ackUnread:   ackUnreadCursor,
		store:       fileReplyStateStore{path: pendingStorePath(cfg)},
		maxRetries:  cfg.AutoReplyRetries(),
		loadContext: loadReplyContext,
		generate:    generateAutoReply,
		send:        sendAutoReply,
		setActivity: activity.Set,
	}, nil
}

// ensureForumPageOpen 用一次健康探测确认论坛页面能打开。
func ensureForumPageOpen(ctx context.Context) error {
	health, err := Refresh(ctx)
	if err != nil {
		return err
	}
	if health == nil || !health.Enabled {
		return errors.New("互助论坛当前未开放")
	}
	return nil
}

// ackUnreadCursor 在事件已落盘后推进服务端事件游标。
func ackUnreadCursor(ctx context.Context, batch unreadBatch) error {
	return advanceEventCursor(ctx, batch.cfg, batch.acct, batch.Cursor)
}

// sendAutoReply 复用既有写入口提交回复：有评论编号时在回复下引用评论，否则在话题下发言。
func sendAutoReply(ctx context.Context, item unreadReply, body string) error {
	if item.CommentID != "" && item.PostID != "" {
		_, err := CreateComment(ctx, item.PostID, body, item.CommentID, nil)
		return err
	}
	if item.TopicID != "" {
		_, err := Reply(ctx, item.TopicID, body, "")
		return err
	}
	return errors.New("回应消息缺少可回复的目标")
}
