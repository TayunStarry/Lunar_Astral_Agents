package viewer

// watch_branch.go 生活链「看视频」分支的唯一实现。
//
// 新流程固定为：取一个已下载未看的片段（每个视频只有一个 1 分钟随机切片）→
// 对这一段做一次视频理解 → 结合标题、评论、弹幕、时长、标签、简介、UP 主合成
// 「视频印象」→ 基于印象生成 1-2 条评论 → 模型决定是否 @ 1-3 个特别关注 →
// 更新「我刚刚在干的事情」→ 每满 8 个完整视频发一条阶段动态。
//
// 生活链权重分支与对话者工具触发都调用 WatchNextSegment，不复制第二份实现。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/viewer/ViewerRemark"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// WatchOutcome 一次观看片段的结果，供生活链记录与外部观测。
type WatchOutcome struct {
	// Triggered 是否真的看了一个片段；false 时 Reason 说明原因。
	Triggered bool   `json:"triggered"`
	Reason    string `json:"reason,omitempty"`
	// Failed 是否属于真正的观看失败（无片可看、视频已不在列表）；暂停让出不算失败。
	Failed bool `json:"failed,omitempty"`

	BV           string   `json:"bv,omitempty"`
	Title        string   `json:"title,omitempty"`
	Topics       []string `json:"topics,omitempty"`
	SegmentIndex int      `json:"segment_index,omitempty"`
	SegmentTotal int      `json:"segment_total,omitempty"`
	LastSegment  bool     `json:"last_segment,omitempty"`

	// VisualSource 画面理解的来源：frames（真实抽帧）或 material（降级为只凭采集材料）。
	VisualSource string `json:"visual_source,omitempty"`
	Comment      string `json:"comment,omitempty"`
	// CommentCount 本次成功生成并尝试发布的评论条数。
	CommentCount int `json:"comment_count,omitempty"`
	// CommentPosted 评论是否至少发布成功一条。
	CommentPosted bool `json:"comment_posted,omitempty"`
	// Mentioned 发布时是否带上了 @ 特别关注。
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
		return "想看点视频，但没有可看的视频"
	}
	if o.CommentPosted {
		return fmt.Sprintf("看了《%s》，还在评论区留了 %d 句话", o.Title, o.CommentCount)
	}
	return fmt.Sprintf("看了《%s》", o.Title)
}

// WatchNextSegment 看一个待看片段并把评论发到原视频评论区。
// 生活链权重分支与工具触发共用这一条路径。
func WatchNextSegment(ctx context.Context, instruction string) (WatchOutcome, error) {
	return watchWithDeps(ctx, instruction, defaultWatchDeps())
}

func watchWithDeps(ctx context.Context, instruction string, deps watchDeps) (WatchOutcome, error) {
	out := WatchOutcome{}
	if deps.pause() {
		out.Reason = "用户消息优先，这个视频留到下次再看"
		return out, nil
	}

	seg, ok, err := deps.nextSegment()
	if err != nil {
		return out, fmt.Errorf("读取待看视频失败: %w", err)
	}
	if !ok {
		out.Reason = "待看列表里还没有下载好的视频"
		out.Failed = true
		return out, nil
	}
	video, found, err := deps.loadVideo(seg.BV)
	if err != nil {
		return out, fmt.Errorf("读取待看视频信息失败: %w", err)
	}
	if !found {
		out.Reason = fmt.Sprintf("视频 %s 已不在待看列表", seg.BV)
		out.Failed = true
		return out, nil
	}
	segments, err := deps.loadSegments(seg.BV)
	if err != nil {
		return out, fmt.Errorf("读取视频观看进度失败: %w", err)
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
	out.Topics = append([]string(nil), video.Topics...)
	out.SegmentIndex = seg.Index + 1
	out.SegmentTotal = total
	out.LastSegment = lastSegmentIndex(segments) == seg.Index

	// 第一步：只对这一段的本地视频做一次理解；拿不到画面时降级为只凭采集材料。
	visual, degrade, err := deps.understand(ctx, seg, video)
	if err != nil {
		degrade = fmt.Sprintf("本地视频无法解码（%v），这次只能凭采集材料写", err)
		visual = ""
	}
	out.VisualSource = "frames"
	if strings.TrimSpace(visual) == "" {
		out.VisualSource = "material"
	}
	out.Note = joinNote(out.Note, degrade)

	// 第二步：补取播放信息（aid / 时长 / 实时弹幕），失败只降级不阻断。
	media := deps.media(video)
	out.Note = joinNote(out.Note, media.Note)

	// 第三步：合成「视频印象」，它是后续评论的唯一依据。
	impression, impressionNote := deps.impression(ctx, video, media, visual)
	out.Note = joinNote(out.Note, impressionNote)

	// 看完立刻更新状态字段，不等评论发布（发布可能因浏览器慢而耗时）。
	out.Activity = activityText(video.Title)
	deps.setActivity(out.Activity)

	// 第四步：基于视频印象生成 1-2 条评论，并按模型意图决定是否 @ 特别关注。
	publishWatchComments(ctx, deps, video, media, &out, impression, instruction)

	// 第五步：标记已看；一个视频只有这一段，标记成功后必定清理本地片段。
	result, err := deps.markWatched(seg.BV, seg.Index, deps.now().Unix())
	if err != nil {
		logger.Warn("BilibiliViewer", "标记视频已看失败: %v", err)
		out.Note = joinNote(out.Note, "标记视频已看失败: "+err.Error())
		if count, cErr := deps.countCompleted(); cErr == nil {
			out.CompletedVideos = count
		}
	} else {
		out.CompletedVideos = result.CompletedVideos
		if cErr := deps.removeClip(seg); cErr != nil {
			logger.Warn("BilibiliViewer", "清理已看完的本地视频失败: %v", cErr)
			out.Note = joinNote(out.Note, "清理本地视频失败: "+cErr.Error())
		}
	}

	// 第六步：阶段动态机制保持不变，仍按累计完整视频数触发。
	out.DynamicPosted, err = deps.milestone(ctx, out.CompletedVideos)
	if err != nil {
		out.Note = joinNote(out.Note, err.Error())
	}

	logger.Info("BilibiliViewer", "[看视频] 《%s》完成，画面来源=%s，评论 %d 条（发布=%v，@=%v），累计完整视频=%d",
		out.Title, out.VisualSource, out.CommentCount, out.CommentPosted, out.Mentioned, out.CompletedVideos)
	return out, nil
}

// publishWatchComments 基于视频印象生成 1-2 条评论并发布。
//
// 候选清单来自本次浏览器会话的「特别关注」；模型为每条评论分别给人数（0-3）
// 与可选的名字/序号。链侧逐条按清单校验：认不出的丢弃，全部无效时按该条人数
// 回退随机挑选；各条评论互不影响。
func publishWatchComments(ctx context.Context, deps watchDeps, video viewerstore.WatchVideo, media watchMedia, out *WatchOutcome, impression, instruction string) {
	material := strings.TrimSpace(impression)
	if material == "" {
		out.Note = joinNote(out.Note, "视频印象为空，跳过评论")
		return
	}
	if instruction = strings.TrimSpace(instruction); instruction != "" {
		material += "\n\n对话者的额外要求：" + truncate(instruction, 400)
	}

	// 打开一次会话：先读特别关注当候选，评论生成后再复用同一会话发布。
	// 会话开启失败不阻断评论生成，只是无法发布与 @。
	session, openErr := deps.openMention()
	if openErr != nil {
		logger.Warn("BilibiliViewer", "B站评论会话开启失败: %v", openErr)
		out.Note = joinNote(out.Note, "B站评论会话开启失败: "+openErr.Error())
	}
	if session != nil {
		defer session.Close()
	}
	var candidates []MentionTarget
	if session != nil {
		candidates = session.Candidates()
	}

	remark, err := deps.remark(ctx, video, material, toMentionCandidates(candidates))
	if err != nil {
		logger.Warn("BilibiliViewer", "视频评论生成失败: %v", err)
		out.Note = joinNote(out.Note, "视频评论生成失败: "+err.Error())
		return
	}
	comments := cleanWatchComments(remark.Comments)
	if len(comments) == 0 {
		out.Note = joinNote(out.Note, "模型没有给出可用评论")
		return
	}
	out.Comment = strings.Join(remarkTexts(comments), " / ")
	out.CommentCount = len(comments)
	if session == nil {
		return
	}
	published := make([]mentionComment, 0, len(comments))
	for _, comment := range comments {
		published = append(published, mentionComment{
			Text:    comment.Text,
			Targets: resolveMentions(candidates, comment.Mention, comment.At, defaultMentionRand),
		})
	}
	posted, mentioned, err := session.Post(video, published, media.Aid)
	out.CommentPosted, out.Mentioned = posted > 0, mentioned
	if err != nil {
		logger.Warn("BilibiliViewer", "视频评论发布失败: %v", err)
		out.Note = joinNote(out.Note, "视频评论发布失败: "+err.Error())
	}
}

// cleanWatchComments 去空白、去重、限长并截到 1-2 条；每条保留自己的 @ 决策。
func cleanWatchComments(raw []ViewerRemark.RemarkComment) []ViewerRemark.RemarkComment {
	out := make([]ViewerRemark.RemarkComment, 0, watchRemarkMaxComments)
	seen := make(map[string]bool, len(raw))
	for _, comment := range raw {
		text := finishComment(strings.TrimSpace(comment.Text), watchRemarkMaxRunes)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		comment.Text = text
		out = append(out, comment)
		if len(out) >= watchRemarkMaxComments {
			break
		}
	}
	return out
}

// remarkTexts 取出评论正文，便于拼接展示与日志。
func remarkTexts(comments []ViewerRemark.RemarkComment) []string {
	out := make([]string, 0, len(comments))
	for _, comment := range comments {
		out = append(out, comment.Text)
	}
	return out
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
func activityText(title string) string {
	return fmt.Sprintf("刚看了哔哩哔哩上的《%s》", title)
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
