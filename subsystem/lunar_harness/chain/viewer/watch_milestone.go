package viewer

// watch_milestone.go 每满 8 个完整视频发一次阶段动态。
//
// 计数只认 viewerstore 的完整视频累计数，不在内存里另存进度：
// 「已发到第几个 8」落盘在 watch_milestone.json，进程重启后据此去重。
// 发布前先写 posting 标记：若上次恰好退出在发布途中，按已发处理，
// 宁可少发一次也不在重启后重复刷动态；生成/发布失败则回滚标记，下次重试。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"LunarSubsystem/LunarHarness/agents/ViewerShare"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

const (
	// videosPerMilestone 每看满多少个完整视频发一次阶段动态。
	videosPerMilestone = 8
	// milestoneFile 阶段动态里程碑文件（相对 LocalDir）。
	milestoneFile = "database/bilibili/watch_milestone.json"
	// milestonePosting 已开始发布、尚未确认完成的中间态。
	milestonePosting = "posting"
	// milestoneDone 已确认发布完成。
	milestoneDone = "done"
)

// milestoneState 阶段动态里程碑的落盘状态。
type milestoneState struct {
	Milestone int    `json:"milestone"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

// postStageDynamic 按累计完整视频数判断是否要发阶段动态；满 8 个只发一次。
func postStageDynamic(ctx context.Context, count int) (bool, error) {
	target, prev, ok := milestoneGate(count)
	if !ok {
		return false, nil
	}
	text, err := stageDynamicText(ctx, count)
	if err != nil || text == "" {
		rollbackMilestone(prev)
		return false, fmt.Errorf("阶段动态生成失败，下次再发: %v", err)
	}
	if err := publishDynamicText(text); err != nil {
		rollbackMilestone(prev)
		return false, fmt.Errorf("阶段动态发布失败: %w", err)
	}
	if err := saveMilestone(milestoneState{Milestone: target, Status: milestoneDone}); err != nil {
		return true, fmt.Errorf("阶段动态已发布，但里程碑落盘失败: %w", err)
	}
	logger.Info("BilibiliViewer", "[看视频] 已满 %d 个完整视频，阶段动态发布完成", count)
	return true, nil
}

// milestoneGate 判断本轮是否需要发阶段动态；需要时先写 posting 标记。
func milestoneGate(count int) (target, prev int, ok bool) {
	target = count / videosPerMilestone
	if target <= 0 {
		return 0, 0, false
	}
	saved := loadMilestone()
	if target <= saved.Milestone {
		return 0, saved.Milestone, false
	}
	if saved.Status == milestonePosting && saved.Milestone == target {
		// 上次在发布途中退出：按已发处理，避免重启后重复发。
		_ = saveMilestone(milestoneState{Milestone: target, Status: milestoneDone})
		return 0, target, false
	}
	if err := saveMilestone(milestoneState{Milestone: target, Status: milestonePosting}); err != nil {
		logger.Warn("BilibiliViewer", "[看视频] 阶段动态标记写入失败: %v", err)
		return 0, saved.Milestone, false
	}
	return target, saved.Milestone, true
}

// rollbackMilestone 回滚到上一个已确认的里程碑，保证下次还能补发。
func rollbackMilestone(prev int) {
	if err := saveMilestone(milestoneState{Milestone: prev, Status: milestoneDone}); err != nil {
		logger.Warn("BilibiliViewer", "[看视频] 阶段动态标记回滚失败: %v", err)
	}
}

// loadMilestone 读取里程碑；文件缺失或损坏按 0 处理。
func loadMilestone() milestoneState {
	raw, ok := persistence.ReadFileBytes(milestoneFile)
	if !ok {
		return milestoneState{Status: milestoneDone}
	}
	var saved milestoneState
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn("BilibiliViewer", "[看视频] 里程碑文件损坏，按 0 处理: %v", err)
		return milestoneState{Status: milestoneDone}
	}
	return saved
}

// saveMilestone 原子写入里程碑。
func saveMilestone(saved milestoneState) error {
	saved.UpdatedAt = time.Now().Unix()
	raw, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	return persistence.WriteFileAtomic(milestoneFile, raw)
}

// stageDynamicText 生成阶段总结动态正文（复用旧版动态智能体）。
func stageDynamicText(ctx context.Context, count int) (string, error) {
	videos, err := viewerstore.LoadApprovedVideos()
	if err != nil {
		return "", err
	}
	material := fmt.Sprintf("最近又看完了一批视频，累计已经 %d 个了。\n最近看完的：\n", count)
	for _, title := range recentCompletedTitles(videos, videosPerMilestone) {
		material += "- " + title + "\n"
	}
	material += "\n写一条阶段性的小总结动态，像随手发在 B 站的一条日常动态。"
	text, err := ViewerShare.Dynamic(ctx, ViewerShare.DynamicInput{
		Material: material, Model: modelPrimary,
	})
	if err != nil {
		return "", err
	}
	return normalizeDynamic(text), nil
}

// recentCompletedTitles 最近看完的若干个视频标题（按完成时间倒序）。
func recentCompletedTitles(videos []viewerstore.WatchVideo, limit int) []string {
	done := make([]viewerstore.WatchVideo, 0, len(videos))
	for _, video := range videos {
		if video.Completed {
			done = append(done, video)
		}
	}
	sort.Slice(done, func(i, j int) bool { return done[i].CompletedAt > done[j].CompletedAt })
	if limit > 0 && len(done) > limit {
		done = done[:limit]
	}
	titles := make([]string, 0, len(done))
	for _, video := range done {
		if title := video.Title; title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}

// publishDynamicText 发布一条 B 站动态（沿用旧版发布通道）。
func publishDynamicText(text string) error {
	session, err := openBilibiliBrowser()
	if err != nil {
		return err
	}
	defer session.close()
	if err := session.ensureLogin(); err != nil {
		return err
	}
	_, err = session.publishDiary(text)
	return err
}
