package viewer

// watchlist.go 生活链触发后的「待看列表」补齐管线。
//
// 四项触发条件全部满足才执行：
//  1. 生活链被触发（EnsureWatchlist 由生活链在事件触发后调用，天然满足）；
//  2. Bilibili 页面能成功开启；
//  3. B 站账号已登录；
//  4. 待看视频列表为空。
//
// 五步严格按序执行：筛候选（24 个）→ 抽视频印象 → 全部审核 → 过审入队 → 切片下载。
// 生活链接线方式（本批次只提供入口，不改 chain/life）：
//
//	result, err := viewer.EnsureWatchlist(ctx)
//	if err != nil { logger.Warn("BilibiliViewer", "待看列表补齐失败: %v", err) }
//
// 条件不满足时返回 Triggered=false + Reason，不产生任何写入。

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// EnsureWatchlist 生活链入口：内部完成四项条件判定与五步补齐。
func EnsureWatchlist(ctx context.Context) (WatchlistResult, error) {
	return ensureWatchlist(ctx, true, defaultWatchlistDeps())
}

// ensureWatchlist 带依赖注入的实现；lifeTriggered 由调用方（生活链）传入。
func ensureWatchlist(ctx context.Context, lifeTriggered bool, deps watchlistDeps) (WatchlistResult, error) {
	result := WatchlistResult{}
	if ctx == nil {
		ctx = context.Background()
	}
	if !lifeTriggered {
		result.Reason = "生活链未触发"
		return result, nil
	}
	empty, err := deps.watchlistEmpty()
	if err != nil {
		return result, fmt.Errorf("检查待看列表失败: %w", err)
	}
	if !empty {
		result.Reason = "待看列表非空"
		return result, nil
	}
	session, err := deps.open()
	if err != nil {
		logger.Warn("BilibiliViewer", "待看列表：Bilibili 页面开启失败，本轮跳过: %v", err)
		result.Reason = "Bilibili 页面开启失败"
		return result, nil
	}
	defer session.close()
	if err := session.ensureLogin(); err != nil {
		logger.Warn("BilibiliViewer", "待看列表：B站账号未登录，本轮跳过: %v", err)
		result.Reason = "B站账号未登录"
		return result, nil
	}
	result.Triggered = true
	budget := newWatchlistBudget(deps.budget.total, deps.budget.perItem, deps.now)
	logger.Info("BilibiliViewer", "待看列表：触发条件满足，开始筛候选（目标 %d 个，总预算 %s）",
		deps.want, budget.total)

	// 第一步：按历史观影记忆筛候选。
	links, err := gatherWatchLinks(session, preferenceKeywords(), deps)
	if err != nil {
		return result, err
	}
	result.Candidates = len(links)

	// 第二步：逐一打开候选，抽取标题、简介、作者 UID/昵称与评论，组成视频印象。
	// 采集只用总预算的 2/3，剩余留给审核与下载；到点即停，未采集的候选不产生任何写入。
	items := make([]*watchItem, 0, len(links))
	for index, link := range links {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if budget.collectExpired() {
			stopForBudget(&result, len(links)-index, "采集")
			break
		}
		itemCtx, cancel := budget.itemContext(ctx)
		item, err := collectWatchItem(itemCtx, session, link, deps)
		cancel()
		if err != nil {
			if budget.collectExpired() {
				stopForBudget(&result, len(links)-index, "采集")
				break
			}
			logger.Warn("BilibiliViewer", "待看列表：候选 %d/%d 采集失败（跳过）: %v", index+1, len(links), err)
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s：%v", link.Href, err))
			continue
		}
		items = append(items, item)
		result.Impressions++
	}
	logger.Info("BilibiliViewer", "待看列表：视频印象完成 %d/%d", result.Impressions, result.Candidates)

	// 第三步：先完成全部审核，再进入任何写库动作；预算耗尽的候选直接放弃，不留半条记录。
	reviewed := 0
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if budget.expired() {
			stopForBudget(&result, len(items)-reviewed, "审核")
			break
		}
		reviewCtx, cancel := budget.itemContext(ctx)
		reviewWatchItem(reviewCtx, item, deps)
		cancel()
		reviewed++
		if item.approved {
			result.Approved++
			continue
		}
		result.Rejected++
		logger.Info("BilibiliViewer", "待看列表：审核未通过《%s》：%s", item.record.Info.Title, item.rejectReason)
	}
	items = items[:reviewed]
	logger.Info("BilibiliViewer", "待看列表：审核完成，通过 %d 个、未通过 %d 个", result.Approved, result.Rejected)

	// 第四步：过审视频入队（BV 幂等），此时仍为 pending，等片段落库后才置 approved。
	inserted, err := persistWatchItems(items, deps)
	if err != nil {
		return result, fmt.Errorf("审核结果入库失败: %w", err)
	}
	result.Enqueued = inserted

	// 第五步：按审核给出的切片数切 1~5 个不重叠片段并下载；预算到点后未下载的顺延到下一轮。
	downloadWatchItems(ctx, items, deps, &result, budget)
	logger.Info("BilibiliViewer", "待看列表：补齐完成，候选 %d｜印象 %d｜过审 %d｜入队 %d｜可播放 %d｜失败 %d｜预算耗尽 %v｜顺延 %d",
		result.Candidates, result.Impressions, result.Approved, result.Enqueued, result.Downloaded,
		result.Failed, result.BudgetExhausted, result.Skipped)
	return result, nil
}

// stopForBudget 记录「因预算耗尽提前结束」并累计被放弃的候选数。
func stopForBudget(result *WatchlistResult, skipped int, phase string) {
	result.BudgetExhausted = true
	result.Skipped += skipped
	logger.Warn("BilibiliViewer", "待看列表：%s阶段时间预算用尽，提前结束，放弃 %d 个候选（已完成的成果保留）", phase, skipped)
}

// preferenceKeywords 汇总历史观影记忆里的偏好词。
//
// 数据源选择：记忆库里的旧观影条目已迁走，viewer.db 中的 viewer_profile（选片口味）、
// viewer_stance（评论倾向）与 viewer_observation（近期观感主题）是唯一权威的历史观影记忆；
// 配置文件里的长期偏好作为兜底补充。
func preferenceKeywords() []string {
	lists := make([][]string, 0, 6)
	if profile, _, ok, err := viewerstore.LoadProfile(); err == nil && ok {
		lists = append(lists, splitDesireWords(profile))
	}
	if stance, _, ok, err := viewerstore.LoadStance(); err == nil && ok {
		lists = append(lists, splitDesireWords(stance))
	}
	if observations, err := viewerstore.LoadObservations(); err == nil {
		for _, observation := range observations {
			lists = append(lists, splitDesireWords(observation.Topics))
		}
	}
	lists = append(lists, LoadConfig().PreferenceKeywords)

	out := make([]string, 0, 8)
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, raw := range list {
			word := raw
			if word == "" || seen[word] {
				continue
			}
			seen[word] = true
			out = append(out, word)
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

// gatherWatchLinks 从首页与关键词搜索结果里凑够候选，并排除已看过/已在待看列表里的视频。
//
// 带「片段下载失败」原因的 rejected 行不算永久历史：允许重新进入候选，下一轮重试下载。
func gatherWatchLinks(session watchSession, keywords []string, deps watchlistDeps) ([]videoLink, error) {
	want := deps.want
	if want <= 0 {
		want = watchlistWanted
	}
	pool, err := session.collectVideoLinks(want * 3)
	if err != nil {
		return nil, fmt.Errorf("读取首页视频卡片失败: %w", err)
	}
	for _, keyword := range pickSearchKeywords(keywords, searchKeywordTries) {
		if len(dedupeLinks(pool)) >= want*2 {
			break
		}
		target := "https://search.bilibili.com/all?keyword=" + url.QueryEscape(keyword)
		if err := session.navigate(target, 25*time.Second); err != nil {
			continue
		}
		session.wait(2 * time.Second)
		found, findErr := session.collectVideoLinks(want)
		if findErr != nil {
			continue
		}
		pool = append(pool, found...)
	}
	cfg := Config{PreferenceKeywords: keywords, DesireKeywords: keywords}
	pool = filterAndScore(dedupeLinks(pool), cfg)
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].Score > pool[j].Score })

	seen := seenSnapshot()
	out := make([]videoLink, 0, want)
	for _, link := range pool {
		bv := bvidOf(link.Href)
		if bv == "" || seen[bv] {
			continue
		}
		existing, ok, err := deps.loadVideo(bv)
		if err != nil {
			return nil, fmt.Errorf("查询待看列表失败: %w", err)
		}
		if ok && !retryableWatchVideo(existing) {
			continue
		}
		out = append(out, link)
		if len(out) >= want {
			break
		}
	}
	return out, nil
}

// retryableWatchVideo 判断一条待看记录能否在本轮重新尝试。
//
// 两类可重试：
//   - rejected + 下载失败前缀：片段没切出来、或本轮预算不足被明确顺延；
//   - pending：已入队但「置为可播放」没写成（进程中断、库写入异常或顺延原因也写失败），
//     这行既不可播，也不会被 approved/rejected 两条路径救回，只能重试。
func retryableWatchVideo(video viewerstore.WatchVideo) bool {
	switch video.Status {
	case viewerstore.WatchStatusRejected:
		return strings.HasPrefix(video.RejectReason, watchlistDownloadFailPrefix)
	case viewerstore.WatchStatusPending:
		return true
	}
	return false
}

// collectWatchItem 打开一个候选视频页并抽取完整「视频印象」。
func collectWatchItem(ctx context.Context, session watchSession, link videoLink, deps watchlistDeps) (*watchItem, error) {
	bv := bvidOf(link.Href)
	if bv == "" {
		return nil, fmt.Errorf("候选链接缺少 BV 号")
	}
	info, err := deps.fetchInfo(bv)
	if err != nil {
		return nil, fmt.Errorf("读取视频信息失败: %w", err)
	}
	if strings.TrimSpace(info.Title) == "" {
		info.Title = link.Title
	}
	// 打开视频页：既满足「逐一打开」的要求，也让评论接口带上当前账号上下文。
	if err := session.navigate(link.Href, 30*time.Second); err != nil {
		logger.Warn("BilibiliViewer", "待看列表：打开视频页失败（改用接口采集）: %v", err)
	}
	session.wait(1200 * time.Millisecond)
	record := &videoRecord{
		Info:     info,
		Comments: readCommentPages(session, info.Aid, watchlistCommentLimit),
	}
	record.Impression = deps.impress(ctx, record)
	return &watchItem{record: record}, nil
}

// reviewWatchItem 用独立审核智能体给出通过/不通过与原因；模型调用发生在智能体内部。
// 审核给出的切片数一并留在 item 上，下载阶段直接沿用，不再自行随机。
func reviewWatchItem(ctx context.Context, item *watchItem, deps watchlistDeps) {
	review := deps.review(ctx, item.record)
	item.approved = review.Pass
	item.slices = review.Slices
	item.rejectReason = strings.TrimSpace(review.Reason)
	if item.approved {
		item.rejectReason = ""
		return
	}
	if item.rejectReason == "" {
		item.rejectReason = "审核未通过"
	}
}

// readCommentPages 分页读取评论，单条超长直接硬截断；抽不到接口数据时退回 DOM 读取。
func readCommentPages(reader commentReader, aid int64, limit int) []commentItem {
	if aid <= 0 || limit <= 0 {
		return nil
	}
	out := make([]commentItem, 0, limit)
	seen := make(map[string]bool, limit)
	for page := 1; page <= collectPageCount && len(out) < limit; page++ {
		items, err := reader.readCommentsAPIPage(1, aid, page, collectPageSize, false)
		if err != nil || len(items) == 0 {
			break
		}
		for _, item := range items {
			key := item.User + "\x00" + item.Content
			if seen[key] {
				continue
			}
			seen[key] = true
			item.Content = hardTruncate(item.Content, collectCommentMaxRunes)
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
	}
	if len(out) == 0 {
		if dom, err := reader.readComments(limit); err == nil {
			for _, item := range dom {
				item.Content = hardTruncate(item.Content, collectCommentMaxRunes)
				out = append(out, item)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}
