package viewer

// auto_respond.go 生活链触发后的 B 站自动响应。
//
// 三项触发条件全部满足才执行：
//  1. 生活链被触发（AutoRespondOnBilibili 由生活链在事件触发后调用，天然满足）；
//  2. Bilibili 页面能成功开启；
//  3. B 站账号已登录。
//
// 三类回应来源分别统计：@我的消息、我发布的视频评论区、我发布的动态评论区。
//
// 生活链接线方式（本批次只提供入口，不改 chain/life 的固定编排与权重表）：
//
//	result, err := viewer.AutoRespondOnBilibili(ctx)
//	if err != nil {
//	    logger.Warn("BilibiliViewer", "B 站自动响应失败: %v", err)
//	}
//
// 条件不满足时返回 Triggered=false + Reason，不产生任何写入，也不更新活动状态。
// 失败条目先落盘再尝试发布：失败留在队列等待下一轮，连续失败达到上限后记录原因
// 并放弃，单条失败不阻塞其余条目。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// AutoRespondOnBilibili 生活链入口：内部完成三项条件判定与三类自动回应。
func AutoRespondOnBilibili(ctx context.Context) (AutoRespondResult, error) {
	return autoRespondOnBilibili(ctx, true, defaultAutoRespondDeps())
}

// autoRespondOnBilibili 带依赖注入的实现；lifeTriggered 由调用方（生活链）传入。
func autoRespondOnBilibili(ctx context.Context, lifeTriggered bool, deps autoRespondDeps) (AutoRespondResult, error) {
	result := AutoRespondResult{}
	if ctx == nil {
		ctx = context.Background()
	}
	if !lifeTriggered {
		result.Reason = "生活链未触发"
		return result, nil
	}
	if deps.maxAttempts < 1 {
		deps.maxAttempts = 1
	}

	session, err := deps.open()
	if err != nil {
		logger.Warn("BilibiliViewer", "自动响应：Bilibili 页面开启失败，本轮跳过: %v", err)
		result.Reason = "Bilibili 页面开启失败"
		return result, nil
	}
	defer session.close()
	if err := session.ensureLogin(); err != nil {
		logger.Warn("BilibiliViewer", "自动响应：B站账号未登录，本轮跳过: %v", err)
		result.Reason = "B站账号未登录"
		return result, nil
	}
	result.Triggered = true

	queue, err := deps.loadQueue()
	if err != nil {
		return result, fmt.Errorf("读取待回应队列失败: %w", err)
	}

	// 先把消息中心的新条目并入队列并落盘，再开始发布：中途崩溃也不会丢条目。
	items, fetchErr := session.fetchInboxAllItems()
	if fetchErr != nil {
		logger.Warn("BilibiliViewer", "自动响应：读取 B 站消息中心失败（先重试本地待回应条目）: %v", fetchErr)
	} else {
		added := 0
		for _, item := range deps.filter(items) {
			source := autoRespondSource(item)
			if source == "" {
				continue
			}
			if queue.enqueue(source, item) {
				added++
			}
		}
		logger.Info("BilibiliViewer", "自动响应：消息中心 %d 条，新入队 %d 条，队列共 %d 条", len(items), added, len(queue.Pending))
		if err := deps.saveQueue(queue); err != nil {
			return result, fmt.Errorf("保存待回应队列失败: %w", err)
		}
	}

	processErr := processRespondQueue(ctx, deps, session, &queue, &result)
	if saveErr := deps.saveQueue(queue); saveErr != nil && processErr == nil {
		processErr = fmt.Errorf("保存待回应队列失败: %w", saveErr)
	}
	deps.setActivity(AutoRespondActivityText(result))
	logger.Info("BilibiliViewer", "自动响应完成：@我 %d/%d、视频评论 %d/%d、动态评论 %d/%d（成功/取到），失败 %d",
		result.At.Replied, result.At.Found,
		result.Video.Replied, result.Video.Found,
		result.Dynamic.Replied, result.Dynamic.Found,
		result.Failed())
	return result, processErr
}

// processRespondQueue 逐条生成并发布；单条失败只累加计数，不阻塞其余条目。
func processRespondQueue(ctx context.Context, deps autoRespondDeps, session autoRespondSession, queue *respondQueue, result *AutoRespondResult) error {
	var saveErr error
	save := func() {
		if err := deps.saveQueue(*queue); err != nil && saveErr == nil {
			saveErr = err
		}
	}
	for index := 0; index < len(queue.Pending); {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := queue.Pending[index]
		stat := result.statOf(entry.Source)
		if stat == nil {
			queue.removeAt(index)
			save()
			continue
		}
		stat.Found++

		text, err := deps.generate(entry.Item)
		if err == nil {
			err = deps.reply(session, entry.Item, text)
		}
		if err == nil {
			stat.Replied++
			deps.markSeen(entry.Item)
			queue.removeAt(index)
			save()
			logger.Info("BilibiliViewer", "自动响应已发布 → %s（%s）", entry.Item.User, entry.Source)
			continue
		}

		entry.Attempts++
		entry.LastError = truncate(err.Error(), autoRespondErrorRunes)
		if entry.Attempts >= deps.maxAttempts {
			// 达到上限：记录原因并放弃，同时标记已处理，避免下一轮被消息中心重新入队。
			queue.removeAt(index)
			queue.drop(entry, entry.LastError)
			deps.markSeen(entry.Item)
			stat.Dropped++
			save()
			logger.Warn("BilibiliViewer", "自动响应放弃 → %s（%s，尝试 %d 次）: %v",
				entry.Item.User, entry.Source, entry.Attempts, err)
			continue
		}
		queue.Pending[index] = entry
		stat.Retrying++
		save()
		logger.Warn("BilibiliViewer", "自动响应失败，保留到下一轮 → %s（%s，%d/%d）: %v",
			entry.Item.User, entry.Source, entry.Attempts, deps.maxAttempts, err)
		index++
	}
	return saveErr
}

// AutoRespondActivityText 处理完成后的活动状态文案；生活链第 3 步也复用它，
// 保证同一件事只有一套措辞。
func AutoRespondActivityText(result AutoRespondResult) string {
	replied := result.Replied()
	failed := result.Failed()
	switch {
	case replied > 0:
		text := fmt.Sprintf("刚在 B 站回了 %d 条消息（%s）", replied, respondBreakdown(result))
		if failed > 0 {
			text += fmt.Sprintf("，还有 %d 条没发出去，下次再试", failed)
		}
		return text
	case failed > 0:
		return "刚去 B 站回消息，但一条也没发出去，下次再试"
	default:
		return "刚去 B 站翻了翻消息，没有需要回应的"
	}
}

// respondBreakdown 三类来源的成功条数明细。
func respondBreakdown(result AutoRespondResult) string {
	return fmt.Sprintf("@我 %d 条、视频评论 %d 条、动态评论 %d 条",
		result.At.Replied, result.Video.Replied, result.Dynamic.Replied)
}
