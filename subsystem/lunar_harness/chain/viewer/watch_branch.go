package viewer

// watch_branch.go 生活链「看视频」分支的唯一实现。
//
// 流程固定：取一个已下载未看的片段 → 本地解码播放（抽帧 + 多模态理解）→
// 普通片段按概率写观感评论，最后一个片段必定写收束评论并 @ 1-3 个特别关注 →
// 更新「我刚刚在干的事情」→ 每满 8 个完整视频发一条阶段动态。
//
// 生活链权重分支与对话者工具触发都调用 WatchNextSegment，不复制第二份实现。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// WatchOutcome 一次观看片段的结果，供生活链记录与外部观测。
type WatchOutcome struct {
	// Triggered 是否真的看了一个片段；false 时 Reason 说明原因。
	Triggered bool   `json:"triggered"`
	Reason    string `json:"reason,omitempty"`

	BV           string `json:"bv,omitempty"`
	Title        string `json:"title,omitempty"`
	SegmentIndex int    `json:"segment_index,omitempty"`
	SegmentTotal int    `json:"segment_total,omitempty"`
	LastSegment  bool   `json:"last_segment,omitempty"`

	// VisualSource 画面理解的来源：frames（真实抽帧）或 impression（降级）。
	VisualSource string `json:"visual_source,omitempty"`
	Comment      string `json:"comment,omitempty"`
	// CommentPosted 观感评论是否发布成功。
	CommentPosted bool `json:"comment_posted,omitempty"`
	// Mentioned 收束评论是否带上了 @ 特别关注。
	Mentioned bool `json:"mentioned,omitempty"`
	// Activity 本次写入的状态字段文案。
	Activity string `json:"activity,omitempty"`
	// CompletedVideos 当前累计看完的完整视频数。
	CompletedVideos int `json:"completed_videos,omitempty"`
	// DynamicPosted 本次是否发出了阶段动态。
	DynamicPosted bool `json:"dynamic_posted,omitempty"`
	// Note 降级、发布失败等需要说明的情况。
	Note string `json:"note,omitempty"`
}

// RecordText 生活记录用的自然语言结果。
func (o WatchOutcome) RecordText() string {
	if !o.Triggered {
		if strings.TrimSpace(o.Reason) != "" {
			return o.Reason
		}
		return "想看点视频，但没有可看的片段"
	}
	if o.LastSegment {
		if o.CommentPosted {
			return fmt.Sprintf("把《%s》看完啦，还在评论区留了一句", o.Title)
		}
		return fmt.Sprintf("把《%s》看完啦", o.Title)
	}
	return fmt.Sprintf("刚看完《%s》的一个片段（第 %d/%d 段）", o.Title, o.SegmentIndex, o.SegmentTotal)
}

// WatchNextSegment 看一个待看片段并把观感评论发到原视频评论区。
// 生活链权重分支与工具触发共用这一条路径。
func WatchNextSegment(ctx context.Context, instruction string) (WatchOutcome, error) {
	return watchWithDeps(ctx, instruction, defaultWatchDeps())
}

func watchWithDeps(ctx context.Context, instruction string, deps watchDeps) (WatchOutcome, error) {
	out := WatchOutcome{}
	if deps.pause() {
		out.Reason = "用户消息优先，这一片段留到下次再看"
		return out, nil
	}

	seg, ok, err := deps.nextSegment()
	if err != nil {
		return out, fmt.Errorf("读取待看片段失败: %w", err)
	}
	if !ok {
		out.Reason = "待看列表里还没有下载好的片段"
		return out, nil
	}
	video, found, err := deps.loadVideo(seg.BV)
	if err != nil {
		return out, fmt.Errorf("读取视频印象失败: %w", err)
	}
	if !found {
		out.Reason = fmt.Sprintf("片段 %s 对应的视频已不在待看列表", seg.BV)
		return out, nil
	}
	segments, err := deps.loadSegments(seg.BV)
	if err != nil {
		return out, fmt.Errorf("读取片段列表失败: %w", err)
	}
	if strings.TrimSpace(video.Title) == "" {
		video.Title = "这段视频"
	}
	total := len(segments)
	if total == 0 {
		total = seg.Index + 1
	}

	out.Triggered = true
	out.BV = seg.BV
	out.Title = video.Title
	out.SegmentIndex = seg.Index + 1
	out.SegmentTotal = total
	out.LastSegment = lastSegmentIndex(segments) == seg.Index

	// 播放即本地解码抽帧：拿不到画面时降级为只凭视频印象写，并在结果里说明。
	visual, degrade, err := deps.understand(ctx, seg, video, total)
	if err != nil {
		degrade = fmt.Sprintf("本地片段无法解码（%v），这次只能凭视频印象写", err)
		visual = ""
	}
	out.VisualSource = "frames"
	if strings.TrimSpace(visual) == "" {
		out.VisualSource = "impression"
	}
	out.Note = joinNote(out.Note, degrade)

	// 看完立刻更新状态字段，不等评论发布（发布可能因浏览器慢而耗时）。
	out.Activity = activityText(video.Title, out.SegmentIndex, total, out.LastSegment)
	deps.setActivity(out.Activity)

	commentAttempted := publishWatchRemark(ctx, deps, video, &out, visual, instruction)

	result, err := deps.markWatched(seg.BV, seg.Index, deps.now().Unix())
	if err != nil {
		logger.Warn("BilibiliViewer", "标记片段已看失败: %v", err)
		out.Note = joinNote(out.Note, "标记片段已看失败: "+err.Error())
		if count, cErr := deps.countCompleted(); cErr == nil {
			out.CompletedVideos = count
		}
	} else {
		out.CompletedVideos = result.CompletedVideos
		// 片段看过且（评论已发或本次未触发评论）就清理；评论尝试失败时保留片段，沿用原有约定。
		if out.CommentPosted || !commentAttempted {
			if cErr := deps.removeClip(seg); cErr != nil {
				logger.Warn("BilibiliViewer", "清理已看完的本地片段失败: %v", cErr)
				out.Note = joinNote(out.Note, "清理本地片段失败: "+cErr.Error())
			}
		}
	}

	out.DynamicPosted, err = deps.milestone(ctx, out.CompletedVideos)
	if err != nil {
		out.Note = joinNote(out.Note, err.Error())
	}

	logger.Info("BilibiliViewer", "[看视频] 《%s》第 %d/%d 段完成，画面来源=%s，评论发布=%v，累计完整视频=%d",
		out.Title, out.SegmentIndex, out.SegmentTotal, out.VisualSource, out.CommentPosted, out.CompletedVideos)
	return out, nil
}

// publishWatchRemark 按档位发布评论：普通片段概率触发，最后一个片段必发收束评论。
// 返回本次是否尝试过评论（未命中概率返回 false），供清理策略区分「没触发」与「发起失败」。
func publishWatchRemark(ctx context.Context, deps watchDeps, video viewerstore.WatchVideo, out *WatchOutcome, visual, instruction string) bool {
	closing := out.LastSegment
	if !closing && !deps.rollRemark() {
		logger.Info("BilibiliViewer", "[看视频] 《%s》第 %d/%d 段未命中评论概率，只更新状态",
			video.Title, out.SegmentIndex, out.SegmentTotal)
		return false
	}
	material := remarkMaterial(video, *out, visual)
	if instruction = strings.TrimSpace(instruction); instruction != "" {
		material += "对话者的额外要求：" + truncate(instruction, 400) + "\n"
	}
	comment, err := deps.remark(ctx, video, material)
	if err != nil {
		logger.Warn("BilibiliViewer", "观感评论生成失败: %v", err)
		out.Note = joinNote(out.Note, "观感评论生成失败: "+err.Error())
		return true
	}
	if comment = strings.TrimSpace(comment); comment == "" {
		return true
	}
	out.Comment = comment
	posted, mentioned, err := deps.postComment(video, comment, closing)
	out.CommentPosted, out.Mentioned = posted, mentioned
	if err != nil {
		logger.Warn("BilibiliViewer", "观感评论发布失败: %v", err)
		out.Note = joinNote(out.Note, "观感评论发布失败: "+err.Error())
	}
	return true
}

// lastSegmentIndex 片段列表里的最后一个序号（列表为空返回 -1）。
func lastSegmentIndex(segments []viewerstore.WatchSegment) int {
	last := -1
	for _, seg := range segments {
		if seg.Index > last {
			last = seg.Index
		}
	}
	return last
}

// activityText 「我刚刚在干的事情」的自然文案。
func activityText(title string, index, total int, last bool) string {
	if last {
		return fmt.Sprintf("刚把《%s》的最后一个片段看完", title)
	}
	return fmt.Sprintf("刚看完《%s》的一个片段（第 %d/%d 段）", title, index, total)
}

// remarkMaterial 拼观感评论的输入：视频印象 + 片段位置 + 画面理解。
func remarkMaterial(video viewerstore.WatchVideo, out WatchOutcome, visual string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "视频：《%s》\n", video.Title)
	if name := strings.TrimSpace(video.AuthorName); name != "" {
		fmt.Fprintf(&b, "UP主：%s\n", name)
	}
	if intro := strings.TrimSpace(video.Intro); intro != "" {
		fmt.Fprintf(&b, "简介：%s\n", truncate(intro, 400))
	}
	if len(video.Comments) > 0 {
		b.WriteString("评论区摘录：\n")
		for i, comment := range video.Comments {
			if i >= 5 {
				break
			}
			if comment = strings.TrimSpace(comment); comment != "" {
				fmt.Fprintf(&b, "- %s\n", truncate(comment, 120))
			}
		}
	}
	fmt.Fprintf(&b, "片段位置：第 %d/%d 段", out.SegmentIndex, out.SegmentTotal)
	if out.LastSegment {
		b.WriteString("（这是整支视频的最后一个片段）")
	}
	b.WriteString("\n")
	if visual = strings.TrimSpace(visual); visual != "" {
		fmt.Fprintf(&b, "这一段的画面：\n%s\n", visual)
	} else {
		b.WriteString("这一段的画面：拿不到画面。\n")
	}
	return b.String()
}

// joinNote 追加说明，自动去重空串。
func joinNote(current, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	return current + "；" + next
}
