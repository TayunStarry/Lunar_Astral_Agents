package BilibiliViewer

// manager.go 哔哩观影管理器：启动阶段同步执行，观看阶段由观影体系 Tick 驱动。
//
// 生命周期：
//  1. StartLifeRound / RunInstruction 打开浏览器、生成关键词、采集候选、生成印象、排序、审核评价，
//     然后构建 Tick 计划表并把浏览器留给后续 Tick；
//  2. 统一 Tick 调度器在观影体系命中时调用 RunOneStep，每次执行一个原子步骤；
//  3. 全部步骤完成后发布动态、写入记忆、关闭浏览器、扣除一次额度。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// lifeManager 进程内唯一的观影管理器。
type lifeManager struct {
	mu      sync.Mutex
	cfg     Config
	job     *runner // 非空表示有一轮观影正在进行（可能处于等待 Tick 的状态）
	running bool    // 启动阶段或某个 Tick 步骤正在执行
}

// runner 一轮观影的运行时状态（跨 Tick 保留）。
type runner struct {
	cfg         Config
	browser     *webSession
	instruction string
	mood        string
	startedAt   time.Time

	mid      int64
	progress string

	videos    []*videoRecord // 采集到的候选（已按综合匹配度排序）
	approved  []*videoRecord // 通过审核的视频（按综合匹配度顺序观看）
	steps     []pipelineStep // Tick 计划表
	stepIndex int
	stepRetry int

	activities  []lifeActivity
	replied     int
	dynamicText string
	dynID       string
	finished    bool
}

// reloadConfigLocked 重新读取 bilibili_life.json（调用方需持有 m.mu）。
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

// stateDir 返回哔哩观影的落盘目录（已看/去重共用）。
func stateDir() string {
	return filepath.Join(localRoot(), "database", "bilibili_life")
}

// RunInstruction 启动一轮哔哩观影（同步完成启动阶段，观看阶段交给 Tick）。
func RunInstruction(instruction string) (string, error) {
	return runInstruction(instruction, "")
}

// StartLifeRound 生活体系入口：不接收对话者自然语言指令，只按心情/配置/记忆选片。
func StartLifeRound(moodLabel string) (string, error) {
	return runInstruction("", moodLabel)
}

func runInstruction(instruction, mood string) (string, error) {
	manager.mu.Lock()
	if manager.job != nil || manager.running {
		manager.mu.Unlock()
		return "哔哩观影正在忙，等这一轮看完再试", nil
	}
	manager.reloadConfigLocked()
	cfg := manager.cfg
	if allow, reason := dispatchBeforeLifeStarts(cfg); !allow {
		manager.mu.Unlock()
		if strings.TrimSpace(reason) == "" {
			reason = "插件未给出原因"
		}
		return "本次观影被 LTP9 插件取消（" + reason + "）", nil
	}
	r := newRunner(cfg, instruction)
	r.mood = strings.TrimSpace(mood)
	manager.running = true
	manager.mu.Unlock()

	if strings.TrimSpace(instruction) != "" {
		logger.Info("BilibiliViewer", "哔哩观影收到任务提示: %s", truncate(instruction, 120))
	}
	text, err := r.startup()

	manager.mu.Lock()
	manager.running = false
	if err != nil || r.finished {
		manager.job = nil
	} else {
		manager.job = r
	}
	manager.mu.Unlock()
	return text, err
}

// Run 同步执行一次启动阶段，供测试/脚本使用。
func Run() error {
	_, err := RunInstruction("")
	return err
}

// newRunner 归一化配置并创建一轮运行状态。
func newRunner(cfg Config, instruction string) *runner {
	if cfg.RepliesPerRound <= 0 {
		cfg.RepliesPerRound = defaultRepliesPerRound
	}
	return &runner{
		cfg:         cfg,
		instruction: strings.TrimSpace(instruction),
		startedAt:   time.Now(),
		progress:    "正在打开 B 站浏览器…",
	}
}

// HasPendingWork 是否还有等待 Tick 的观影步骤。
func HasPendingWork() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.job != nil && manager.job.stepIndex < len(manager.job.steps)
}

// RunOneStep 同步执行一个原子步骤；由 Tick 调度器在自己的 goroutine 中调用。
func RunOneStep(ctx context.Context) error {
	manager.mu.Lock()
	job := manager.job
	if job == nil || manager.running || job.stepIndex >= len(job.steps) {
		manager.mu.Unlock()
		return nil
	}
	if err := ctx.Err(); err != nil {
		manager.mu.Unlock()
		return err
	}
	manager.running = true
	manager.mu.Unlock()

	var panicErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				panicErr = fmt.Errorf("观影步骤异常: %v", recovered)
				logger.Error("BilibiliViewer", "Tick 步骤异常: %v", recovered)
			}
		}()
		job.runNextStep()
	}()

	manager.mu.Lock()
	done := job.stepIndex >= len(job.steps)
	manager.running = false
	manager.mu.Unlock()

	if done {
		job.finishRound()
		manager.mu.Lock()
		if manager.job == job {
			manager.job = nil
		}
		manager.mu.Unlock()
	}
	return panicErr
}

// hasActiveJob 报告是否存在等待 Tick 的观影任务（启动阶段与观看阶段都算）。
func hasActiveJob() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.job != nil
}

// resultText 把启动阶段的结果整理成工具回复（简洁陈述风格）。
func (r *runner) resultText() string {
	if r == nil {
		return "[系统提示]哔哩观影已完成本次流程"
	}
	approved := 0
	thoughts := make([]string, 0, 2)
	for _, video := range r.approved {
		if video == nil {
			continue
		}
		approved++
		if len(thoughts) < 2 && strings.TrimSpace(video.Impression.Summary) != "" {
			thoughts = append(thoughts, truncate(video.Impression.Summary, 40))
		}
	}
	if approved == 0 {
		return "这次没找到合适的视频，已经先记下来了。"
	}
	summary := fmt.Sprintf("这次挑了 %d 个视频，准备慢慢看", approved)
	if r.replied > 0 {
		summary += fmt.Sprintf("，顺手回了 %d 条消息", r.replied)
	}
	if len(thoughts) > 0 {
		summary += "。第一眼感觉：" + strings.Join(thoughts, "；")
	}
	return summary + "。"
}

// logActivity 追加一条本轮互动记录（写记忆用）。
func (r *runner) logActivity(activity lifeActivity) {
	r.activities = append(r.activities, activity)
}

// setProgress 更新运行进度（管理器状态与日志共用）。
func (r *runner) setProgress(text string) {
	if r == nil {
		return
	}
	r.progress = text
	logger.Info("BilibiliViewer", "%s", text)
}

// watchedTitles 本轮已观看的视频标题（写记忆用）。
func (r *runner) watchedTitles() []string {
	out := make([]string, 0, len(r.approved))
	for _, video := range r.approved {
		if video != nil && strings.TrimSpace(video.Info.Title) != "" {
			out = append(out, video.Info.Title)
		}
	}
	return out
}

// cleanup 关闭浏览器并清理临时视频（切片只是内存里的时间区间，无需删文件）。
func (r *runner) cleanup() {
	if r == nil {
		return
	}
	if r.browser != nil {
		r.browser.close()
		r.browser = nil
	}
	for _, video := range r.videos {
		if video == nil {
			continue
		}
		if video.LocalPath != "" {
			_ = os.Remove(video.LocalPath)
			video.LocalPath = ""
		}
		video.Segments = nil
	}
}

// clampFloat 把浮点值夹紧到 [lo, hi]
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
