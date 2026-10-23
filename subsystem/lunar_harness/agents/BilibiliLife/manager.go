package BilibiliLife

// 生活链管理器：可中断、可续跑、自动调度、消息闸门与进度通道。
//
// 与 goja 对话链的约定：
//   - 生活运行时，Go 侧未读消息到达 → 请求中断；中断完成前 pullContext 拿不到消息；
//   - 对话链完成且 10 秒内没有新未读 → 自动从断点继续；
//   - 进度标题通过 bilibiliLifeProgress() 暴露给对话者运行时。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	module "LunarSubsystem/LunarHarness/engine/multimodal"
)

var errPaused = errors.New("life paused for dialogue")

type phase string

const (
	phaseLogin         phase = "login"
	phaseInbox         phase = "inbox"
	phaseInboxReply    phase = "inbox_reply"
	phaseSelect        phase = "select"
	phaseDownload      phase = "download"
	phaseUnderstand    phase = "understand"
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
}

type lifeState struct {
	Version      int            `json:"version"`
	Phase        phase          `json:"phase"`
	VideoIndex   int            `json:"video_index"`
	Videos       []*videoRecord `json:"videos,omitempty"`
	Current      *videoRecord   `json:"current,omitempty"`
	ShareDone    bool           `json:"share_done,omitempty"` // 全轮唯一的 @分享评论是否已发
	DynID        string         `json:"dyn_id,omitempty"`
	DiaryHandle  string         `json:"diary_handle,omitempty"` // 本轮日记落地的句柄（@D.xxx）
	DiaryLines   int            `json:"diary_lines,omitempty"`  // 本轮日记段的总行数（标题+正文）
	ReplyTargets []replyTarget  `json:"reply_targets,omitempty"`
	ReplyIndex   int            `json:"reply_index"`
	RepliesUsed  int            `json:"replies_used,omitempty"` // 已用掉的回应额度（评论/动态/私信共享）
	ReplyChecked bool           `json:"reply_checked"`
	// 未读消息（回复我的 / @我的）在进入看视频前先处理
	InboxChecked bool        `json:"inbox_checked,omitempty"`
	InboxTargets []inboxItem `json:"inbox_targets,omitempty"`
	InboxIndex   int         `json:"inbox_index"`
	AuditRejects int         `json:"audit_rejects,omitempty"` // 本轮被静态画面审核拒绝的视频数
	Progress     string      `json:"progress"`
	StartedAt    string      `json:"started_at"`
	Forced       bool        `json:"forced"`
}

type runner struct {
	cfg     Config
	browser *webSession
	state   *lifeState
}

type autoStats struct {
	Date       string `json:"date"`
	Count      int    `json:"count"`
	LastAutoAt string `json:"last_auto_at"`
}

type lifeManager struct {
	mu            sync.Mutex
	cfg           Config
	cfgStamp      time.Time // 上次读取 bilibili_life.json 的修改时间（用于热重载）
	runner        *runner
	running       bool
	paused        bool
	interrupted   bool
	interruptedAt time.Time
	progress      string
	resumeTimer   *time.Timer
}

var (
	manager       = &lifeManager{}
	dialogueBusy  atomic.Bool
	unreadCounter func() int
	stateFile     = func() string { return filepath.Join(localRoot(), "database", "bilibili_life", "state.json") }
	autoStatsFile = func() string { return filepath.Join(localRoot(), "database", "bilibili_life", "auto.json") }
	// processStartedAt 进程启动时刻（包初始化即赋值）
	processStartedAt = time.Now()
)

// startupLifeGrace 启动后的自动生活静默期。
//
// 这段时间内自动调度不进入生活，只有 TUI 强制触发能开始：服务刚起来时模型、前端、
// 浏览器都还没就绪，而且断点文件里的上次自动时间可能早已过期，不该一启动就弹 B 站窗口。
const startupLifeGrace = 10 * time.Minute

// autoLifeGraceActive 是否仍处在启动静默期内。
func autoLifeGraceActive() bool { return time.Since(processStartedAt) < startupLifeGrace }

// reloadConfigLocked 重新读取 bilibili_life.json 并记录文件修改时间（调用方需持有 m.mu）。
func (m *lifeManager) reloadConfigLocked() {
	m.cfg = LoadConfig()
	if info, err := os.Stat(configPath()); err == nil {
		m.cfgStamp = info.ModTime()
	}
}

// refreshConfigLocked 配置文件的修改时间变了就热重载（调用方需持有 m.mu）。
//
// 为什么需要：自动生活以前只在内存配置为空时读一次文件，用户改完 mention_targets /
// preference_keywords 后，自动轮次会一直用旧值（表现为「永远只 @ 同一个人」），
// 必须重启或 TUI 强制触发才生效。按 mtime 判断，改完下一轮就生效。
func (m *lifeManager) refreshConfigLocked() {
	info, err := os.Stat(configPath())
	if err != nil || !info.ModTime().After(m.cfgStamp) {
		return
	}
	m.reloadConfigLocked()
	logger.Info("BilibiliLife", "检测到 bilibili_life.json 变更，已重新加载配置")
}

func localRoot() string {
	dir := strings.TrimSpace(*config.LocalDir)
	if dir == "" {
		return "local_data"
	}
	return dir
}

// SetUnreadCounter 注册宿主未读数（由 lunar_loop 在初始化时注入）。
func SetUnreadCounter(fn func() int) { unreadCounter = fn }

// SetDialogueBusy 由 goja 对话链同步“是否正在推理”，TUI 强制生活据此延迟。
func SetDialogueBusy(busy bool) { dialogueBusy.Store(busy) }

// Start 由 TUI 调用（forced=true）：跳过自动间隔/每日上限，不占自动次数。
func Start() (string, error) { return manager.start(true) }

// Run 同步执行一次，供测试/脚本使用。
func Run() error {
	manager.mu.Lock()
	if manager.running {
		manager.mu.Unlock()
		return fmt.Errorf("已有一次生活正在运行")
	}
	manager.reloadConfigLocked()
	// 生活开始前事件：插件可取消本次生活（取消仍消耗一次本日额度并计为完成）
	if allow, reason := dispatchBeforeLifeStarts(manager.cfg, true); !allow {
		consumeLifeQuota(reason)
		return nil
	}
	manager.runner = newRunner(manager.cfg, true)
	manager.running = true
	manager.paused = false
	manager.interrupted = false
	manager.progress = "正在准备生活…"
	done := make(chan error, 1)
	go manager.loop(manager.runner, done)
	manager.mu.Unlock()
	return <-done
}

func (m *lifeManager) start(forced bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return "已有一次生活正在运行，等它结束或被打断", nil
	}
	if dialogueBusy.Load() {
		return "对话正在推理，等这一轮回答完再开始生活", nil
	}
	if unreadCounter != nil && unreadCounter() > 0 {
		return "还有未读消息，先处理对话再开始生活", nil
	}
	if m.paused && m.runner != nil {
		m.resumeLocked()
		return "已继续上次没走完的生活", nil
	}
	m.reloadConfigLocked()
	// 生活开始前事件：插件可取消本次生活（取消仍消耗一次本日额度并计为完成）
	if allow, reason := dispatchBeforeLifeStarts(m.cfg, forced); !allow {
		consumeLifeQuota(reason)
		return "生活开始前被 LTP9 插件取消（已计为完成一次生活）", nil
	}
	m.runner = newRunner(m.cfg, forced)
	m.running = true
	m.paused = false
	m.interrupted = false
	m.progress = "正在准备生活…"
	go m.loop(m.runner, nil)
	return "已启动：正在打开 B 站浏览器", nil
}

func newRunner(cfg Config, forced bool) *runner {
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
	logger.Info("BilibiliLife", "本轮选片偏好：参考词 %d 个；过滤词 %d 个；@目标 %d 个；目标视频 %d 个；每视频 %d 个切片；仅在动态呼叫用户=%v",
		len(cfg.PreferenceKeywords), len(cfg.BlockedKeywords), len(cfg.MentionTargets),
		cfg.VideosPerRound, cfg.SlicesPerVideo, cfg.onlyCallUserInDynamic())
	return &runner{
		cfg: cfg,
		state: &lifeState{
			Version:   1,
			Phase:     phaseLogin,
			Progress:  "正在打开 B 站浏览器…",
			StartedAt: time.Now().Format(time.RFC3339),
			Forced:    forced,
		},
	}
}

func (m *lifeManager) loop(r *runner, done chan error) {
	for {
		if m.shouldPause() {
			m.pause(r)
			if done != nil {
				done <- nil
			}
			return
		}
		finished, err := r.step(m)
		if errors.Is(err, errPaused) {
			m.pause(r)
			if done != nil {
				done <- nil
			}
			return
		}
		if err != nil {
			m.fail(r, err)
			if done != nil {
				done <- err
			}
			return
		}
		if finished {
			m.finish(r)
			if done != nil {
				done <- nil
			}
			return
		}
	}
}

func (m *lifeManager) shouldPause() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interrupted
}

func (m *lifeManager) setProgress(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = text
	if m.runner != nil && m.runner.state != nil {
		m.runner.state.Progress = text
	}
}

func (m *lifeManager) pause(r *runner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	m.paused = true
	m.interrupted = false
	// 保留最后一步的进度（如“看到了 2/3”），对话者运行时仍能看到停在哪里
	r.saveState()
	logger.Info("BilibiliLife", "生活已暂停（当前阶段 %s，进度：%s），等对话结束后续跑", r.state.Phase, m.progress)
}

func (m *lifeManager) fail(r *runner, err error) {
	logger.Error("BilibiliLife", "本次刷视频失败: %v", err)
	// 生活结束前事件（纯触发器）：失败也是一种结束，插件可据此做收尾
	dispatchBeforeLifeEnds(r, "failed", err.Error())
	m.mu.Lock()
	m.running = false
	m.paused = false
	m.interrupted = false
	m.progress = ""
	m.runner = nil
	m.mu.Unlock()
	// 日记已落文档但本轮中途失败：回执照发，月华仍能按句柄回看这段日记
	notifyDiaryReceipt(r)
	r.cleanup()
}

func (m *lifeManager) finish(r *runner) {
	// 生活结束前事件（纯触发器）：正常完成，插件可据此做收尾
	dispatchBeforeLifeEnds(r, "finished", "")
	m.mu.Lock()
	m.running = false
	m.paused = false
	m.interrupted = false
	m.progress = ""
	m.runner = nil
	m.mu.Unlock()
	notifyDiaryReceipt(r)
	r.cleanup()
	logger.Info("BilibiliLife", "本次生活完成")
}

// notifyDiaryReceipt 本轮写过日记文档时，经注册回调把固定回执送进月华未读队列。
// 回调在 finish/fail（running 已复位）之后触发：EnqueueMessage 内部的未读中断
// 请求不会再次打断已结束的生活。
func notifyDiaryReceipt(r *runner) {
	if r == nil || r.state == nil || r.state.DiaryHandle == "" || diaryNotifier == nil {
		return
	}
	diaryNotifier(diaryReceiptText(r.state.DiaryHandle, r.state.DiaryLines))
}

func (m *lifeManager) resumeLocked() {
	if m.runner == nil {
		return
	}
	m.running = true
	m.paused = false
	m.interrupted = false
	m.progress = "继续上次没走完的生活…"
	go m.loop(m.runner, nil)
}

// NotifyUnread 宿主收到未读消息时调用：生活在跑就请求中断，并取消续跑计时。
func NotifyUnread() {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.resumeTimer != nil {
		manager.resumeTimer.Stop()
		manager.resumeTimer = nil
	}
	if manager.running && !manager.paused {
		manager.interrupted = true
		manager.interruptedAt = time.Now()
		logger.Info("BilibiliLife", "检测到未读消息，生活将在当前步骤后暂停")
	}
}

// HoldMessages true 时 pullContext 暂不交付消息（生活还没停到断点）。
func HoldMessages() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if !manager.running || manager.paused {
		return false
	}
	// 兜底竞态：生活刚启动时队列里已经有未读，但 Enqueue 的 NotifyUnread
	// 发生在 Start 之前。这里看到未读就立刻补上中断请求，避免对话把消息抢走。
	if !manager.interrupted && unreadCounter != nil && unreadCounter() > 0 {
		manager.interrupted = true
		manager.interruptedAt = time.Now()
		logger.Info("BilibiliLife", "发现未读消息，补发生活中断请求")
	}
	if !manager.interrupted {
		return false
	}
	// 兜底：如果生活卡在一个不可中断的长调用里超过 5 分钟，不能再让对话饿死。
	if !manager.interruptedAt.IsZero() && time.Since(manager.interruptedAt) > 5*time.Minute {
		logger.Warn("BilibiliLife", "生活中断等待超过 5 分钟，临时放开消息拉取避免对话饿死")
		return false
	}
	return true
}

// Progress 对话者运行时拉取的生活进度；结束/空闲返回空串。
func Progress() string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.runner == nil {
		return ""
	}
	if manager.runner.state.Phase == phaseDone {
		return ""
	}
	return manager.progress
}

// NotifyDialogueIdle 对话链完成一次推理后调用；10 秒静默后续跑暂停中的生活。
func NotifyDialogueIdle() {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if !manager.paused || manager.running || manager.runner == nil {
		return
	}
	if unreadCounter != nil && unreadCounter() > 0 {
		return
	}
	if manager.resumeTimer != nil {
		manager.resumeTimer.Stop()
	}
	manager.resumeTimer = time.AfterFunc(10*time.Second, func() {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if !manager.paused || manager.running || manager.runner == nil {
			return
		}
		if unreadCounter != nil && unreadCounter() > 0 {
			return
		}
		logger.Info("BilibiliLife", "对话已空闲 10 秒，继续未完成的生活")
		manager.resumeLocked()
	})
}

// AutoTick 由 goja 在空闲 tick 调用；到点则启动自动生活。
func AutoTick() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.running || manager.paused || dialogueBusy.Load() {
		return false
	}
	if unreadCounter != nil && unreadCounter() > 0 {
		return false
	}
	// 启动静默期：自动调度不进入生活，只有 TUI 强制触发（Start）能开始
	if autoLifeGraceActive() {
		return false
	}
	// 配置文件改动后立即热重载：否则改 mention_targets / preference_keywords
	// 要等重启才生效，自动轮次会一直按旧配置跑
	manager.refreshConfigLocked()
	cfg := manager.cfg
	if !cfg.autoEnabled() {
		return false
	}
	stats := loadAutoStats()
	today := time.Now().Format("2006-01-02")
	if stats.Date != today {
		stats.Date = today
		stats.Count = 0
	}
	if stats.Count >= cfg.DailyLimit {
		return false
	}
	if stats.LastAutoAt == "" {
		// 首次启动不立刻开浏览器；从启动时刻起算一个完整间隔，
		// 避免每次重启应用都突然弹 B 站窗口。想立刻看用 TUI 强制触发。
		stats.LastAutoAt = time.Now().Format(time.RFC3339)
		saveAutoStats(stats)
		return false
	}
	if last, err := time.Parse(time.RFC3339, stats.LastAutoAt); err == nil {
		if time.Since(last) < time.Duration(cfg.IntervalHours*float64(time.Hour)) {
			return false
		}
	}
	manager.cfg = cfg
	// 生活开始前事件：插件可取消本次自动生活（取消仍消耗一次本日额度并计为完成）
	if allow, reason := dispatchBeforeLifeStarts(cfg, false); !allow {
		consumeLifeQuota(reason)
		return false
	}
	manager.runner = newRunner(cfg, false)
	manager.running = true
	manager.paused = false
	manager.interrupted = false
	manager.progress = "自动生活开始…"
	stats.Count++
	stats.LastAutoAt = time.Now().Format(time.RFC3339)
	saveAutoStats(stats)
	logger.Info("BilibiliLife", "自动生活触发（今天第 %d/%d 次）", stats.Count, cfg.DailyLimit)
	go manager.loop(manager.runner, nil)
	return true
}

func loadAutoStats() autoStats {
	var stats autoStats
	raw, err := os.ReadFile(autoStatsFile())
	if err == nil {
		_ = json.Unmarshal(raw, &stats)
	}
	return stats
}

func saveAutoStats(stats autoStats) {
	if err := os.MkdirAll(filepath.Dir(autoStatsFile()), 0o755); err != nil {
		return
	}
	raw, _ := json.MarshalIndent(stats, "", "  ")
	_ = os.WriteFile(autoStatsFile(), raw, 0o644)
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

// step 执行一个可中断的小步骤；返回 finished=true 表示整轮结束。
func (r *runner) step(m *lifeManager) (bool, error) {
	if m.shouldPause() {
		return false, errPaused
	}
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
		if err := r.browser.ensureLoginInterruptible(m.shouldPause); err != nil {
			if errors.Is(err, errPaused) {
				r.saveState()
				return false, errPaused
			}
			return false, err
		}
		s.Phase = phaseInbox
		r.saveState()
		return false, m.pauseIfNeeded()

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
			logger.Info("BilibiliLife", "没有需要回应的未读消息，开始刷视频")
			s.Phase = phaseSelect
		} else {
			logger.Info("BilibiliLife", "发现 %d 条未读消息，先回应再刷视频", len(s.InboxTargets))
			s.Phase = phaseInboxReply
		}
		r.saveState()
		return false, m.pauseIfNeeded()

	case phaseInboxReply:
		// 逐条回应，每条一个可中断小步骤；额度与动态回复共享
		if s.InboxIndex >= len(s.InboxTargets) || s.RepliesUsed >= r.cfg.RepliesPerRound {
			if remaining := len(s.InboxTargets) - s.InboxIndex; remaining > 0 {
				logger.Info("BilibiliLife", "本轮回应额度已用完，剩余 %d 条未读留到下一轮", remaining)
			}
			s.Phase = phaseSelect
			r.saveState()
			return false, m.pauseIfNeeded()
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
		}
		s.InboxIndex++
		s.RepliesUsed++
		r.saveState()
		return false, m.pauseIfNeeded()

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
		// 选中即记为已看：下一轮不再推同一条（下载/理解失败也不重试同一条）
		markSeen(bvid)
		s.Current = &videoRecord{Link: link, Info: info}
		s.Phase = phaseDownload
		r.saveState()
		return false, m.pauseIfNeeded()

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
		// 内容审核：静态/高度雷同画面极易触发模型无限思考循环，命中即舍弃重选。
		// 该视频已在 phaseSelect 计入已看记录；这里不增加 VideoIndex，不占观看额度。
		rejected, auditErr := auditVideoStasis(local)
		if auditErr != nil {
			// 审核器故障不阻断生活链：记录告警后放行
			logger.Warn("BilibiliLife", "视频内容审核失败（放行）: %v", auditErr)
		} else if rejected {
			s.AuditRejects++
			limit := auditRejectLimit(r.cfg.VideosPerRound)
			logger.Info("BilibiliLife", "《%s》(%s) 未通过内容审核（第 %d/%d 次），重新选片（不占观看额度）",
				cur.Info.Title, cur.Info.Bvid, s.AuditRejects, limit)
			_ = os.Remove(local)
			cur.LocalPath = ""
			cur.Segments = nil
			s.Current = nil
			if s.AuditRejects >= limit {
				return false, fmt.Errorf("连续 %d 个视频未通过静态画面审核（阈值 %d），放弃本轮", s.AuditRejects, limit)
			}
			s.Phase = phaseSelect
			r.saveState()
			return false, m.pauseIfNeeded()
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
		return false, m.pauseIfNeeded()

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
			return false, m.pauseIfNeeded()
		}
		total := len(cur.Segments)
		m.setProgress(fmt.Sprintf("月华正在看第 %d/%d 组《%s》，看到了 %d/%d", s.VideoIndex+1, r.cfg.VideosPerRound, cur.Info.Title, cur.SliceIndex+1, total))
		text, err := understandOneSlice(cur.Info, cur.Segments[cur.SliceIndex], cur.SliceIndex+1, total)
		if err != nil {
			logger.Warn("BilibiliLife", "第 %d/%d 片理解失败: %v", cur.SliceIndex+1, total, err)
			text = ""
		}
		cur.Summaries = append(cur.Summaries, text)
		cur.SliceIndex++
		r.saveState()
		return false, m.pauseIfNeeded()

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
		s.Phase = phaseCommentsWrite
		r.saveState()
		return false, m.pauseIfNeeded()

	case phaseCommentsWrite:
		cur := s.Current
		if cur == nil {
			s.Phase = phaseSelect
			return false, nil
		}
		// 每个视频固定配 1 条评论：正文基于视频理解 + 已读到的热门评论
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
			return false, m.pauseIfNeeded()
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
		} else {
			s.Phase = phaseShare
		}
		r.saveState()
		return false, m.pauseIfNeeded()

	case phaseDecide:
		// 至少看完 1 个才允许停下（min=1）；月华自己决定「就到此为止」时提前收工
		if s.VideoIndex < videoMinCount || wantMoreVideos(s.Videos, r.cfg.VideosPerRound) {
			s.Phase = phaseSelect
			r.saveState()
			return false, m.pauseIfNeeded()
		}
		s.Phase = phaseShare
		m.setProgress(fmt.Sprintf("看完 %d 个视频，这轮就到这里…", s.VideoIndex))
		logger.Info("BilibiliLife", "看完 %d/%d 个视频，月华决定这轮不再继续刷", s.VideoIndex, r.cfg.VideosPerRound)
		r.saveState()
		return false, m.pauseIfNeeded()

	case phaseShare:
		// 一轮只允许挑选其中一个视频发 @分享评论：本阶段在整轮只走一次
		if s.ShareDone {
			s.Phase = phaseDiary
			return false, nil
		}
		// 「仅在动态中呼叫用户」开启时：完全跳过「从多个视频里挑一条 + 生成 @评论」，
		// 改为在 phaseDiary 的动态末尾另起一行 @ 目标用户。
		if r.cfg.onlyCallUserInDynamic() {
			logger.Info("BilibiliLife", "已开启「仅在动态中呼叫用户」，跳过视频 @分享评论")
			s.ShareDone = true
			s.Phase = phaseDiary
			r.saveState()
			return false, m.pauseIfNeeded()
		}
		// 没配置 mention_targets 就不发 @ 分享：开源项目不内置任何 @ 对象，
		// 避免别人部署后 @ 到作者或无关账号。
		names := r.cfg.mentionNames()
		if len(names) == 0 {
			logger.Info("BilibiliLife", "未配置 mention_targets，跳过 @分享评论")
			s.ShareDone = true
			s.Phase = phaseDiary
			r.saveState()
			return false, m.pauseIfNeeded()
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
			err = r.browser.postComment(featured.Info.Aid, text)
		}
		if err != nil {
			logger.Warn("BilibiliLife", "@分享评论（@%s）失败: %v", strings.Join(names, "、"), err)
		} else {
			logger.Info("BilibiliLife", "@分享评论已发布（@%s）：%s", strings.Join(names, "、"), text)
		}
		s.ShareDone = true
		s.Phase = phaseDiary
		r.saveState()
		return false, m.pauseIfNeeded()

	case phaseDiary:
		m.setProgress("正在写这一轮的生活日记…")
		title, brief, err := generateDiaryForVideos(s.Videos)
		if err != nil {
			return false, err
		}
		// 日记列举本轮看过的每个视频，不只是发 @ 的那一条
		urls := make([]string, 0, len(s.Videos))
		for _, video := range s.Videos {
			if video != nil {
				urls = append(urls, video.Info.URL)
			}
		}
		// 同一份文案两处落地：先写句柄文档（发动态失败也不丢），再原样发动态
		handle, lines, err := appendDiaryDocument(title, urls, brief)
		if err != nil {
			return false, err
		}
		s.DiaryHandle = handle
		s.DiaryLines = lines
		// 开启「仅在动态中呼叫用户」时，动态正文末尾单独一行 @ 目标用户；
		// 关闭时沿用原有逻辑（呼叫内容已在 phaseShare 的 @分享评论里）。
		var callNames []string
		if r.cfg.onlyCallUserInDynamic() {
			callNames = r.cfg.mentionNames()
		}
		dynamic := buildDynamicTextWithCall(title, urls, brief, callNames)
		dynID, err := r.browser.publishDiary(dynamic)
		if err != nil {
			return false, err
		}
		s.DynID = dynID
		s.Phase = phaseReplies
		r.saveState()
		return false, m.pauseIfNeeded()

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
			return false, m.pauseIfNeeded()
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
			}
			s.RepliesUsed++
			s.ReplyIndex++
			r.saveState()
			return false, m.pauseIfNeeded()
		}
		s.Phase = phaseDone
		r.saveState()
		return false, nil

	case phaseDone:
		return true, nil
	}
	return false, fmt.Errorf("未知生活阶段: %s", s.Phase)
}

func (m *lifeManager) pauseIfNeeded() error {
	if m.shouldPause() {
		return errPaused
	}
	return nil
}

// pickFeaturedVideo 从本轮看过的视频里挑出唯一的主角视频（用于 @分享评论与动态）。
//
// 选评论区收集得最多的那条：讨论最热闹的视频最值得分享，也让「只发一条 @消息」
// 有明确依据，而不是随机。条数相同时取先看的那条，保证断点续跑时选择稳定。
func pickFeaturedVideo(videos []*videoRecord) *videoRecord {
	var best *videoRecord
	for _, video := range videos {
		if video == nil {
			continue
		}
		if best == nil || len(video.Comments) > len(best.Comments) {
			best = video
		}
	}
	return best
}
