package BilibiliLife

// 观影者管理器：由对话者通过 viewer 工具按流程同步调用。
//
// 三种流程（Flow）：
//   standard      回复未读评论 + 看视频与评论 + 写日记动态 + 回复动态评论
//   watch_comment 看视频与评论 + 写日记动态（不处理未读、不回复动态评论）
//   reply_only    仅回复消息
//
// 动态只承载日记；分享消息与 @ 用户由 phaseShare 的视频评论完成，
// 是否构建分享评论由工具参数 share 控制（不传默认 true）。
// 每次调用同步跑完并返回日记/结果文本；不再有自动调度、TUI 强制触发或对话-生活交替。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/loopguard"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

type phase string

const (
	phaseLogin         phase = "login"
	phaseInbox         phase = "inbox"
	phaseInboxReply    phase = "inbox_reply"
	phaseSelect        phase = "select"
	phaseDownload      phase = "download"
	phaseUnderstand    phase = "understand"
	phaseFallback      phase = "fallback"
	phaseCommentsRead  phase = "comments_read"
	phaseCommentsWrite phase = "comments_write"
	phaseDecide        phase = "decide"
	phaseShare         phase = "share"
	phaseNextVideo     phase = "next_video"
	phaseDiary         phase = "diary"
	phaseReplies       phase = "replies"
	phaseDone          phase = "done"
)

type videoRecord struct {
	Link          videoLink             `json:"link"`
	Info          videoInfo             `json:"info"`
	LocalPath     string                `json:"local_path,omitempty"`
	Segments      []module.MediaSegment `json:"segments,omitempty"`
	SliceIndex    int                   `json:"slice_index"`
	Summaries     []string              `json:"summaries,omitempty"`
	Summary       string                `json:"summary,omitempty"`
	Comments      []commentItem         `json:"comments,omitempty"`
	NormalWritten int                   `json:"normal_written"`
	// Fallback 画面理解因思考监管重复而放弃：改用标题/封面/简介/评论/弹幕构建侧面理解
	Fallback bool     `json:"fallback,omitempty"`
	Danmaku  []string `json:"danmaku,omitempty"`
	// Risky 风险筛查命中：不评论、不分享、不写进日记动态
	Risky bool `json:"risky,omitempty"`
}

type lifeState struct {
	Version      int            `json:"version"`
	Flow         Flow           `json:"flow"`
	Phase        phase          `json:"phase"`
	VideoIndex   int            `json:"video_index"`
	Videos       []*videoRecord `json:"videos,omitempty"`
	Current      *videoRecord   `json:"current,omitempty"`
	ShareDone    bool           `json:"share_done,omitempty"` // 全轮唯一的分享评论是否已发
	Share        bool           `json:"share,omitempty"`      // 本轮是否构建分享评论（工具参数）
	DynID        string         `json:"dyn_id,omitempty"`
	DiaryHandle  string         `json:"diary_handle,omitempty"` // 本轮日记落地的句柄（@D.xxx）
	DiaryLines   int            `json:"diary_lines,omitempty"`  // 本轮日记段的总行数（标题+正文）
	Replied      int            `json:"replied,omitempty"`      // 本轮成功回复的消息条数
	ReplyTargets []replyTarget  `json:"reply_targets,omitempty"`
	ReplyIndex   int            `json:"reply_index"`
	RepliesUsed  int            `json:"replies_used,omitempty"` // 已用掉的回应额度（评论/动态/私信共享）
	ReplyChecked bool           `json:"reply_checked"`
	// 未读消息（回复我的 / @我的）在进入看视频前先处理
	InboxChecked bool        `json:"inbox_checked,omitempty"`
	InboxTargets []inboxItem `json:"inbox_targets,omitempty"`
	InboxIndex   int         `json:"inbox_index"`
	RiskRejects  int         `json:"risk_rejects,omitempty"` // 本轮被风险筛查拒绝的视频数
	Progress     string      `json:"progress"`
	StartedAt    string      `json:"started_at"`
}

type runner struct {
	cfg         Config
	browser     *webSession
	state       *lifeState
	instruction string // 对话者本轮想法（viewer 工具的 instruction）
}

type lifeManager struct {
	mu      sync.Mutex
	cfg     Config
	runner  *runner
	running bool
}

var (
	manager   = &lifeManager{}
	stateFile = func() string {
		return filepath.Join(localRoot(), "database", "bilibili_life", "state.json")
	}
)

// reloadConfigLocked 重新读取 bilibili_life.json（调用方需持有 m.mu）。
// 每次工具调用都会重读，改完配置无需重启。
func (m *lifeManager) reloadConfigLocked() {
	m.cfg = LoadConfig()
}

func localRoot() string {
	dir := strings.TrimSpace(config.LocalDir)
	if dir == "" {
		return "local_data"
	}
	return dir
}

// RunFlow 同步执行一次观影者流程，返回可直接作为工具回复的文本。
// flow 由对话者通过 viewer 工具的枚举选择；share 决定是否为本轮选中的视频构建分享评论；
// instruction 只作为任务提示（可空）。
func RunFlow(flow Flow, share bool, instruction string) (string, error) {
	if flow == "" {
		flow = FlowStandard
	}

	manager.mu.Lock()
	if manager.running {
		manager.mu.Unlock()
		return "观影者正在忙，等这次结束再试", nil
	}
	manager.reloadConfigLocked()
	cfg := manager.cfg
	if flow.Watches() && watchQuotaExhausted(cfg) {
		manager.mu.Unlock()
		return "今天已经刷了很多视频了，不适合再看了", nil
	}
	if allow, reason := dispatchBeforeLifeStarts(cfg, flow); !allow {
		if flow.Watches() {
			consumeWatchQuota(cfg)
		}
		manager.mu.Unlock()
		if strings.TrimSpace(reason) == "" {
			reason = "插件未给出原因"
		}
		return "本次观影被 LTP9 插件取消（" + reason + "）", nil
	}
	if flow.Watches() {
		consumeWatchQuota(cfg)
	}
	r := newRunner(cfg, flow, share, instruction)
	manager.runner = r
	manager.running = true
	manager.mu.Unlock()

	// 关键词提炼会调用模型，必须在释放 manager.mu 之后执行，避免阻塞其它管理查询
	if flow.Watches() {
		r.prepareDesireKeywords()
	}
	if strings.TrimSpace(instruction) != "" {
		logger.Info("BilibiliLife", "观影者收到任务提示[%s]: %s", flow, truncate(instruction, 120))
	}
	text, err := runSync(r)

	manager.mu.Lock()
	manager.running = false
	manager.runner = nil
	manager.mu.Unlock()
	return text, err
}

// Run 同步执行一次标准流程，供测试/脚本使用。
func Run() error {
	_, err := RunFlow(FlowStandard, false, "")
	return err
}

func newRunner(cfg Config, flow Flow, share bool, instruction string) *runner {
	// 刷视频数量：默认 3 个，硬边界 1~5；切片与回复次数只保证为正数
	if cfg.VideosPerRound <= 0 {
		cfg.VideosPerRound = defaultVideosPerRound
	}
	cfg.VideosPerRound = clampInt(cfg.VideosPerRound, videoMinCount, videoMaxCount)
	if cfg.SlicesPerVideo <= 0 {
		cfg.SlicesPerVideo = defaultSlicesPerVideo
	}
	if cfg.RepliesPerRound <= 0 {
		cfg.RepliesPerRound = defaultRepliesPerRound
	}
	// 对话者的本轮想法覆写配置文件偏好：只在本次运行内生效，不落盘。
	// 具体提炼在 RunFlow 释放 manager 锁之后执行（会调用模型，不能占着锁）。
	logger.Info("BilibiliLife", "观影者流程[%s]：默认参考词 %d 个（可被对话者欲求覆写）；@目标 %d 个；目标视频 %d 个；每视频 %d 个切片；构建分享=%v",
		flow, len(cfg.PreferenceKeywords), len(cfg.MentionTargets), cfg.VideosPerRound, cfg.SlicesPerVideo, share)
	return &runner{
		cfg:         cfg,
		instruction: strings.TrimSpace(instruction),
		state: &lifeState{
			Version:   1,
			Flow:      flow,
			Share:     share,
			Phase:     phaseLogin,
			Progress:  "正在打开 B 站浏览器…",
			StartedAt: time.Now().Format(time.RFC3339),
		},
	}
}

// flow 当前流程（旧断点没有 Flow 字段时按标准流程处理）。
func (r *runner) flow() Flow {
	if r == nil || r.state == nil || r.state.Flow == "" {
		return FlowStandard
	}
	return r.state.Flow
}

// resultText 把本轮结果整理成工具回复：
// 真正刷过视频才返回日记句柄；只回复了评论时返回成功回复的条数。
func (r *runner) resultText() string {
	if r == nil || r.state == nil {
		return "观影者已完成本次流程"
	}
	s := r.state
	if s.VideoIndex > 0 && s.DiaryHandle != "" {
		return fmt.Sprintf("已经刷完一轮视频了，可查询%s|L1-%d 获取看到的内容", s.DiaryHandle, s.DiaryLines)
	}
	if s.Replied > 0 {
		return fmt.Sprintf("已回复 %d 条 B 站评论", s.Replied)
	}
	if r.flow() == FlowReplyOnly {
		return "已回复 0 条 B 站评论"
	}
	if s.VideoIndex > 0 {
		return fmt.Sprintf("看了 %d 个视频，本轮结束", s.VideoIndex)
	}
	return "观影者已完成本次流程"
}

// runSync 同步推进流程，直到完成或失败；返回结果文本。
func runSync(r *runner) (string, error) {
	for step := 0; step < 1000; step++ {
		finished, err := r.step(manager)
		if err != nil {
			abortRun(r, err)
			return r.resultText(), err
		}
		if finished {
			text := r.resultText()
			finishRun(r)
			return text, nil
		}
	}
	err := fmt.Errorf("观影者流程步数超限")
	abortRun(r, err)
	return r.resultText(), err
}

func (m *lifeManager) setProgress(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runner != nil && m.runner.state != nil {
		m.runner.state.Progress = text
	}
}

func finishRun(r *runner) {
	dispatchBeforeLifeEnds(r, "finished", "")
	logger.Info("BilibiliLife", "观影者流程完成[%s]", r.flow())
	r.cleanup()
}

func abortRun(r *runner, err error) {
	logger.Error("BilibiliLife", "观影者流程失败[%s]: %v", r.flow(), err)
	dispatchBeforeLifeEnds(r, "failed", err.Error())
	r.cleanup()
}

func (r *runner) saveState() {
	if r == nil || r.state == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(stateFile()), 0o755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(r.state, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(stateFile(), raw, 0o644); err != nil {
		logger.Warn("BilibiliLife", "保存生活断点失败: %v", err)
	}
}

func (r *runner) cleanup() {
	if r.browser != nil {
		r.browser.close()
		r.browser = nil
	}
	if r.state != nil {
		if r.state.Current != nil {
			removeSegments(r.state.Current.Segments)
			if r.state.Current.LocalPath != "" {
				_ = os.Remove(r.state.Current.LocalPath)
			}
		}
		for _, video := range r.state.Videos {
			removeSegments(video.Segments)
		}
	}
	_ = os.Remove(stateFile())
}

// step 执行流程中的一个小步骤；返回 finished=true 表示整轮结束。
func (r *runner) step(m *lifeManager) (bool, error) {
	s := r.state
	switch s.Phase {
	case phaseLogin:
		m.setProgress("正在打开 B 站浏览器…")
		if r.browser == nil {
			browser, err := openBilibiliBrowser()
			if err != nil {
				return false, err
			}
			r.browser = browser
		}
		if err := r.browser.ensureLogin(); err != nil {
			return false, err
		}
		if r.flow().includesInbox() {
			s.Phase = phaseInbox
		} else {
			s.Phase = phaseSelect
		}
		r.saveState()
		return false, nil

	case phaseInbox:
		// 必须在选片之前完成：先把评论区追评、动态回复与 @月华 处理掉
		if s.InboxChecked {
			s.Phase = phaseInboxReply
			return false, nil
		}
		m.setProgress("正在检查 B 站未读消息…")
		items, err := r.browser.fetchInboxItems()
		if err != nil {
			logger.Warn("BilibiliLife", "读取 B 站未读消息失败（本轮跳过）: %v", err)
			items = nil
		}
		s.InboxTargets = filterUnreadInbox(items, loadInboxSeen(), time.Now())
		s.InboxIndex = 0
		s.InboxChecked = true
		if len(s.InboxTargets) == 0 {
			logger.Info("BilibiliLife", "没有需要回应的未读消息")
			if r.flow() == FlowReplyOnly {
				s.Phase = phaseDone
			} else {
				s.Phase = phaseSelect
			}
		} else {
			logger.Info("BilibiliLife", "发现 %d 条未读消息，先回应", len(s.InboxTargets))
			s.Phase = phaseInboxReply
		}
		r.saveState()
		return false, nil

	case phaseInboxReply:
		// 逐条回应，每条一个可中断小步骤；额度与动态回复共享
		if s.InboxIndex >= len(s.InboxTargets) || s.RepliesUsed >= r.cfg.RepliesPerRound {
			if remaining := len(s.InboxTargets) - s.InboxIndex; remaining > 0 {
				logger.Info("BilibiliLife", "本轮回应额度已用完，剩余 %d 条未读留到下一轮", remaining)
			}
			if r.flow() == FlowReplyOnly {
				s.Phase = phaseDone
			} else {
				s.Phase = phaseSelect
			}
			r.saveState()
			return false, nil
		}
		item := s.InboxTargets[s.InboxIndex]
		m.setProgress(fmt.Sprintf("正在回应 B 站未读消息 %d/%d…", s.RepliesUsed+1, r.cfg.RepliesPerRound))
		text, err := generateInboxReply(item)
		if err == nil {
			err = r.browser.postInboxReply(item, text)
		}
		if err != nil {
			logger.Warn("BilibiliLife", "回应未读消息失败 → %s: %v", item.User, err)
		} else {
			logger.Info("BilibiliLife", "已回应 B 站未读消息 → %s：%s", item.User, text)
			markInboxSeen(item)
			s.Replied++
		}
		s.InboxIndex++
		s.RepliesUsed++
		r.saveState()
		return false, nil

	case phaseSelect:
		m.setProgress(fmt.Sprintf("正在挑第 %d/%d 组视频…", s.VideoIndex+1, r.cfg.VideosPerRound))
		var link videoLink
		var err error
		// 一次选片内复用同一份已看快照，避免对每个候选重复读盘
		seen := seenSnapshot()
		for attempt := 0; attempt < 3; attempt++ {
			link, err = r.browser.selectVideo(r.cfg, seen)
			if err != nil {
				return false, err
			}
			// 一轮看多个视频时，尽量不要重复选中同一条
			seenAgain := false
			for _, old := range s.Videos {
				if old != nil && old.Link.Href == link.Href {
					seenAgain = true
					break
				}
			}
			if !seenAgain {
				break
			}
		}
		bvid := bvidOf(link.Href)
		if bvid == "" {
			return false, fmt.Errorf("详情页 URL 里没有 BV 号: %s", link.Href)
		}
		info, err := fetchVideoInfo(bvid)
		if err != nil {
			return false, fmt.Errorf("读取视频信息失败: %w", err)
		}
		if info.Title == "" {
			info.Title = link.Title
		}
		// 选中即记为已看：下一轮不再推同一条（风险拒选/下载/理解失败也不重试同一条）
		markSeen(bvid)
		// 风险筛查：标题 + 简介 + 封面，命中即换一条（不占观看额度）
		if verdict := screenVideoRisk(info, nil); verdict.Risky {
			s.RiskRejects++
			limit := riskRejectLimit(r.cfg.VideosPerRound)
			logger.Info("BilibiliLife", "《%s》(%s) 未通过风险筛查（第 %d/%d 次），重新选片：%s",
				info.Title, info.Bvid, s.RiskRejects, limit, verdict.Reason)
			if s.RiskRejects >= limit {
				return false, fmt.Errorf("连续 %d 个视频未通过风险筛查（阈值 %d），放弃本轮", s.RiskRejects, limit)
			}
			s.Phase = phaseSelect
			r.saveState()
			return false, nil
		}
		s.Current = &videoRecord{Link: link, Info: info}
		s.Phase = phaseDownload
		r.saveState()
		return false, nil

	case phaseDownload:
		cur := s.Current
		if cur == nil {
			s.Phase = phaseSelect
			return false, nil
		}
		m.setProgress(fmt.Sprintf("正在下载《%s》…", cur.Info.Title))
		playURL, err := fetchPlayURL(cur.Info.Bvid, cur.Info.Cid, 32)
		if err != nil {
			return false, fmt.Errorf("获取播放直链失败: %w", err)
		}
		seconds := r.cfg.SlicesPerVideo * sliceSec
		if cur.Info.Duration > 0 && cur.Info.Duration < seconds {
			seconds = cur.Info.Duration
		}
		local, err := downloadClip(playURL, cur.Info.Bvid, seconds)
		if err != nil {
			return false, fmt.Errorf("下载视频失败: %w", err)
		}
		segments, err := prepareVideoSegments(local, r.cfg.SlicesPerVideo)
		if err != nil {
			_ = os.Remove(local)
			return false, err
		}
		cur.LocalPath = local
		cur.Segments = segments
		s.Phase = phaseUnderstand
		r.saveState()
		return false, nil

	case phaseUnderstand:
		cur := s.Current
		if cur == nil {
			s.Phase = phaseSelect
			return false, nil
		}
		if cur.SliceIndex >= len(cur.Segments) {
			cur.Summary = combineSliceSummaries(cur.Info, cur.Summaries)
			// 整条视频理解完成：写入视频摘要库（键=视频 URL），供月华事后按 URL 回溯
			storeVideoSummary(cur.Info, cur.Summaries)
			removeSegments(cur.Segments)
			if cur.LocalPath != "" {
				_ = os.Remove(cur.LocalPath)
				cur.LocalPath = ""
			}
			cur.Segments = nil
			s.Phase = phaseCommentsRead
			r.saveState()
			return false, nil
		}
		total := len(cur.Segments)
		m.setProgress(fmt.Sprintf("观影者正在看第 %d/%d 组《%s》，看到了 %d/%d", s.VideoIndex+1, r.cfg.VideosPerRound, cur.Info.Title, cur.SliceIndex+1, total))
		text, err := understandOneSlice(cur.Info, cur.Segments[cur.SliceIndex], cur.SliceIndex+1, total)
		if err != nil {
			if errors.Is(err, loopguard.ErrLoop) {
				// 任一画面触发思考监管重复：放弃整条视频的全部切片理解，改走侧面理解。
				// 同一视频的其他切片画面高度同源，继续逐片重试只会重复卡住。
				logger.Warn("BilibiliLife", "第 %d/%d 片触发思考监管重复输出，放弃《%s》全部画面理解，改用侧面理解",
					cur.SliceIndex+1, total, cur.Info.Title)
				cur.Summaries = nil
				cur.Fallback = true
				removeSegments(cur.Segments)
				cur.Segments = nil
				if cur.LocalPath != "" {
					_ = os.Remove(cur.LocalPath)
					cur.LocalPath = ""
				}
				s.Phase = phaseFallback
				r.saveState()
				return false, nil
			}
			logger.Warn("BilibiliLife", "第 %d/%d 片理解失败: %v", cur.SliceIndex+1, total, err)
			text = ""
		}
		cur.Summaries = append(cur.Summaries, text)
		cur.SliceIndex++
		r.saveState()
		return false, nil

	case phaseFallback:
		// 画面理解已放弃：用标题 + 封面 + 简介 + 热门评论 + 弹幕构建侧面理解，
		// 供后续评论、日记动态与分享评论使用（评论在这里一次读齐，跳过 phaseCommentsRead）。
		cur := s.Current
		if cur == nil {
			s.Phase = phaseSelect
			return false, nil
		}
		if strings.TrimSpace(cur.Summary) != "" {
			s.Phase = phaseCommentsWrite
			r.saveState()
			return false, nil
		}
		m.setProgress(fmt.Sprintf("正在为《%s》整理侧面理解…", cur.Info.Title))
		if len(cur.Comments) == 0 {
			r.browser.scrollToComments()
			comments, err := r.browser.readCommentsAPI(cur.Info.Aid, commentsReadLimit)
			if err != nil || len(comments) == 0 {
				if dom, domErr := r.browser.readComments(commentsReadLimit); domErr == nil {
					comments = dom
				}
			}
			if len(comments) > commentsReadLimit {
				comments = comments[:commentsReadLimit]
			}
			cur.Comments = comments
		}
		cur.Danmaku = fetchDanmaku(cur.Info.Cid, fallbackDanmakuLimit)
		cover := fetchCoverDataURL(cur.Info.Pic)
		cur.Summary = composeSideUnderstanding(cur.Info, cur.Comments, cur.Danmaku, cover)
		// 侧面理解顺手读到了评论：同样做一次风险复查，命中就不留言
		if verdict := screenCommentsRisk(cur.Comments); verdict.Risky {
			cur.Risky = true
			logger.Warn("BilibiliLife", "《%s》评论区未通过风险筛查，跳过评论/分享/日记：%s", cur.Info.Title, verdict.Reason)
		}
		logger.Info("BilibiliLife", "《%s》侧面理解完成：简介 %d 字，评论 %d 条，弹幕 %d 条，封面=%v",
			cur.Info.Title, len([]rune(cur.Info.Desc)), len(cur.Comments), len(cur.Danmaku), cover != "")
		s.Phase = phaseCommentsWrite
		r.saveState()
		return false, nil

	case phaseCommentsRead:
		cur := s.Current
		if cur == nil {
			s.Phase = phaseSelect
			return false, nil
		}
		m.setProgress(fmt.Sprintf("正在读《%s》的 %d 条热门评论…", cur.Info.Title, commentsReadLimit))
		r.browser.scrollToComments()
		comments, err := r.browser.readCommentsAPI(cur.Info.Aid, commentsReadLimit)
		if err != nil || len(comments) == 0 {
			if dom, domErr := r.browser.readComments(commentsReadLimit); domErr == nil {
				comments = dom
			}
		}
		if len(comments) > commentsReadLimit {
			comments = comments[:commentsReadLimit]
		}
		cur.Comments = comments
		// 评论区复查：命中风险就不再留言（视频仍计入已看，但不进分享与日记）
		if verdict := screenCommentsRisk(comments); verdict.Risky {
			cur.Risky = true
			logger.Warn("BilibiliLife", "《%s》评论区未通过风险筛查，跳过评论/分享/日记：%s", cur.Info.Title, verdict.Reason)
		}
		s.Phase = phaseCommentsWrite
		r.saveState()
		return false, nil

	case phaseCommentsWrite:
		cur := s.Current
		if cur == nil {
			s.Phase = phaseSelect
			return false, nil
		}
		// 每个视频固定配 1 条评论：正文基于视频理解 + 已读到的热门评论
		if cur.Risky {
			logger.Info("BilibiliLife", "《%s》存在风险，跳过留言", cur.Info.Title)
			cur.NormalWritten++
			s.Phase = phaseNextVideo
			r.saveState()
			return false, nil
		}
		if cur.NormalWritten < commentsPerVideo {
			m.setProgress(fmt.Sprintf("正在写《%s》的评论…", cur.Info.Title))
			text, err := generateComment(cur.Info, cur.Summary, cur.Comments)
			if err == nil {
				err = r.browser.postComment(cur.Info.Aid, text)
			}
			if err != nil {
				logger.Warn("BilibiliLife", "评论失败: %v", err)
			} else {
				logger.Info("BilibiliLife", "评论已发布：%s", text)
			}
			cur.NormalWritten++
			r.saveState()
			return false, nil
		}
		s.Phase = phaseNextVideo
		return false, nil

	case phaseNextVideo:
		if s.Current != nil {
			s.Videos = append(s.Videos, s.Current)
			s.Current = nil
		}
		s.VideoIndex++
		// 还有目标额度时才让月华在视频之间决定要不要继续；到量就直接收尾
		if s.VideoIndex < r.cfg.VideosPerRound {
			s.Phase = phaseDecide
		} else if s.Share {
			s.Phase = phaseShare
		} else {
			s.Phase = phaseDiary
		}
		r.saveState()
		return false, nil

	case phaseDecide:
		// 至少看完 1 个才允许停下（min=1）；月华自己决定「就到此为止」时提前收工
		if s.VideoIndex < videoMinCount || wantMoreVideos(s.Videos, r.cfg.VideosPerRound) {
			s.Phase = phaseSelect
			r.saveState()
			return false, nil
		}
		if s.Share {
			s.Phase = phaseShare
		} else {
			s.Phase = phaseDiary
		}
		m.setProgress(fmt.Sprintf("看完 %d 个视频，这轮就到这里…", s.VideoIndex))
		logger.Info("BilibiliLife", "看完 %d/%d 个视频，月华决定这轮不再继续刷", s.VideoIndex, r.cfg.VideosPerRound)
		r.saveState()
		return false, nil

	case phaseShare:
		// 一轮只允许挑选其中一个视频发 @分享评论：本阶段在整轮只走一次
		if s.ShareDone {
			s.Phase = phaseDiary
			return false, nil
		}
		// 分享消息与 @ 用户统一由视频评论完成；动态只承载日记。
		// 没配置 mention_targets 就不发 @ 分享：开源项目不内置任何 @ 对象，
		// 避免别人部署后 @ 到作者或无关账号。
		names := r.cfg.mentionNames()
		if len(names) == 0 {
			logger.Info("BilibiliLife", "未配置 mention_targets，跳过 @分享评论")
			s.ShareDone = true
			s.Phase = phaseDiary
			r.saveState()
			return false, nil
		}
		featured := pickFeaturedVideo(s.Videos)
		if featured == nil {
			s.ShareDone = true
			s.Phase = phaseDiary
			r.saveState()
			return false, nil
		}
		m.setProgress(fmt.Sprintf("正在给《%s》发 @分享评论…", featured.Info.Title))
		text, err := generateShareComment(featured.Info, featured.Summary, featured.Comments)
		// 分享时 @ 全部目标（最多 mentionMaxCount 人），而不是随机挑一个
		atText := "@" + strings.Join(names, " @")
		if err == nil {
			text = fmt.Sprintf("%s %s", truncate(strings.TrimSpace(strings.ReplaceAll(text, "@", "")), 60), atText)
			err = r.browser.postCommentWithMentions(featured.Info.Aid, text, r.cfg.mentionTargets())
		}
		if err != nil {
			logger.Warn("BilibiliLife", "@分享评论（@%s）失败: %v", strings.Join(names, "、"), err)
		} else {
			logger.Info("BilibiliLife", "@分享评论已发布（@%s）：%s", strings.Join(names, "、"), text)
		}
		s.ShareDone = true
		s.Phase = phaseDiary
		r.saveState()
		return false, nil

	case phaseDiary:
		// 风险视频不写进公开日记动态（标题与观看感受都可能扩大风险暴露）
		safeVideos := make([]*videoRecord, 0, len(s.Videos))
		for _, video := range s.Videos {
			if video != nil && !video.Risky {
				safeVideos = append(safeVideos, video)
			}
		}
		if len(safeVideos) == 0 {
			logger.Warn("BilibiliLife", "本轮没有通过风险筛查的视频，跳过日记动态")
			s.Phase = phaseDone
			r.saveState()
			return false, nil
		}
		m.setProgress("正在写这一轮的观影日记…")
		title, brief, err := generateDiaryForVideos(safeVideos)
		if err != nil {
			return false, err
		}
		// 日记列举本轮看过的每个安全视频（标题与 URL 同行），不只是发 @ 的那一条
		videos := make([]diaryVideo, 0, len(safeVideos))
		for _, video := range safeVideos {
			videos = append(videos, diaryVideo{Title: video.Info.Title, URL: video.Info.URL})
		}
		handle, lines, err := appendDiaryDocument(title, videos, brief)
		if err != nil {
			return false, err
		}
		s.DiaryHandle = handle
		s.DiaryLines = lines

		// 动态只承载日记；分享消息与 @ 用户由 phaseShare 的视频评论完成。
		if r.flow().publishesDynamic() {
			dynamic := buildDynamicText(title, videos, brief)
			dynID, err := r.browser.publishDiary(dynamic)
			if err != nil {
				return false, err
			}
			s.DynID = dynID
		}

		if r.flow() == FlowStandard {
			s.Phase = phaseReplies
		} else {
			s.Phase = phaseDone
		}
		r.saveState()
		return false, nil

	case phaseReplies:
		if s.ReplyTargets == nil {
			m.setProgress("正在看日记动态的评论…")
			// 刚发完动态评论索引可能还没就绪；只等一次 10 秒，避免整轮卡住。
			if !s.ReplyChecked {
				s.ReplyChecked = true
				r.saveState()
				time.Sleep(10 * time.Second)
			}
			targets, err := r.browser.fetchDynamicReplies(s.DynID)
			if err != nil {
				logger.Warn("BilibiliLife", "读取动态评论失败: %v", err)
				targets = nil
			}
			s.ReplyTargets = targets
			r.saveState()
			if len(targets) == 0 {
				s.Phase = phaseDone
				return false, nil
			}
			return false, nil
		}
		// 回应额度在一轮内共享（评论回复/动态回复/私信回复都从这一份额度里扣）
		if s.ReplyIndex < len(s.ReplyTargets) && s.RepliesUsed < r.cfg.RepliesPerRound {
			target := s.ReplyTargets[s.ReplyIndex]
			budget := r.cfg.RepliesPerRound
			m.setProgress(fmt.Sprintf("正在回复动态评论 %d/%d…", s.RepliesUsed+1, budget))
			text, err := generateReply(target.User, target.Message)
			if err == nil {
				err = r.browser.postDynamicReply(s.DynID, target, text)
			}
			if err != nil {
				logger.Warn("BilibiliLife", "回复 %s 失败: %v", target.User, err)
			} else {
				logger.Info("BilibiliLife", "动态回复已发布 → %s：%s", target.User, text)
				// 记入未读去重表：下一轮消息中心再拉到这条互动时不再重复回复。
				// source/reply 两个键都记，兼容消息中心把这条评论放在不同字段的情况。
				markInboxSeenKeys([]string{
					fmt.Sprintf("reply:17:%s:%s", s.DynID, target.Rpid),
					fmt.Sprintf("source:17:%s:%s", s.DynID, target.Rpid),
				})
				s.Replied++
			}
			s.RepliesUsed++
			s.ReplyIndex++
			r.saveState()
			return false, nil
		}
		s.Phase = phaseDone
		r.saveState()
		return false, nil

	case phaseDone:
		return true, nil
	}
	return false, fmt.Errorf("未知观影流程阶段: %s", s.Phase)
}

// pickFeaturedVideo 从本轮看过的视频里挑出唯一的主角视频（用于 @分享评论与动态）。
//
// 选评论区收集得最多且未命中风险的那条：讨论最热闹的视频最值得分享，也让「只发一条 @消息」
// 有明确依据，而不是随机。条数相同时取先看的那条，保证断点续跑时选择稳定。
func pickFeaturedVideo(videos []*videoRecord) *videoRecord {
	var best *videoRecord
	for _, video := range videos {
		if video == nil || video.Risky {
			continue
		}
		if best == nil || len(video.Comments) > len(best.Comments) {
			best = video
		}
	}
	return best
}
