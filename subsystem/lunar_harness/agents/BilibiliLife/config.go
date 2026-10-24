package BilibiliLife

// 观影者 · B 站真实浏览器版：选片偏好与安全过滤。
//
// 用户参考词与过滤词放在 local_data/bilibili_life.json（已 gitignore），
// 不在代码里硬编码个人偏好；文件缺失时使用安全默认值。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

type MentionTarget struct {
	Name string `json:"name"`
	UID  int64  `json:"uid,omitempty"`
}

// ==== 一轮生活的固定机制参数 ====
//
// 以下取值是「机制」而非「偏好」，因此不做成配置项：
// 每个视频读固定条数的热门评论、固定只写 1 条评论、全轮只允许 1 条 @分享评论、
// 动态正文最多 10 行简述。数量类（刷几个视频、几个切片、回复几条）才开放配置。
const (
	// videoMinCount / videoMaxCount 一轮刷视频数量的硬边界，配置越界时夹紧
	videoMinCount = 1
	videoMaxCount = 5
	// defaultVideosPerRound 默认一轮刷 3 个视频（中途可由月华决定就此停下）
	defaultVideosPerRound = 3
	// defaultSlicesPerVideo 每个视频默认 10 个 60 秒切片
	defaultSlicesPerVideo = 10
	// commentsReadLimit 每个视频固定读 15 条热门评论，用于写评论时参考
	commentsReadLimit = 15
	// commentsPerVideo 每个视频固定配 1 条评论
	commentsPerVideo = 1
	// mentionMaxCount 分享动态时同时 @ 的人数上限（超出按配置顺序截断）
	mentionMaxCount = 5
	// defaultRepliesPerRound 默认一轮回应 5 次消息（评论/动态/私信共用这一份额度）
	defaultRepliesPerRound = 5
	// defaultDailyWatchLimit 默认每天最多调用 6 次刷视频流程
	defaultDailyWatchLimit = 6
	// dynamicSummaryMaxLines 动态正文里的简述最多 10 行
	dynamicSummaryMaxLines = 10
	// fallbackDanmakuLimit 画面理解失败时，侧面理解读取的弹幕条数
	fallbackDanmakuLimit = 30
	// fallbackCoverMaxBytes 侧面理解下载封面的体积上限（4MB），超出则只用文字材料
	fallbackCoverMaxBytes = 4 << 20
	// lifeLoopMaxRetries 文本类模型调用命中重复输出后的额外重试次数；
	// 切片理解不走重试（任一画面命中即放弃整条视频的画面理解）。
	lifeLoopMaxRetries = 3
)

type Config struct {
	// 更倾向看的主题词（标题/卡片文本命中即加分）
	PreferenceKeywords []string `json:"preference_keywords"`
	// @分享评论可以 @ 的用户列表；不再硬编码为某一个人
	MentionTargets []MentionTarget `json:"mention_targets"`

	// 每日可调用观影者刷视频的次数上限（仅统计包含观看视频的流程）
	DailyLimit      int `json:"daily_limit"`
	VideosPerRound  int `json:"videos_per_round"`  // 一轮最多刷几个视频（1-5）
	SlicesPerVideo  int `json:"slices_per_video"`  // 每个视频理解几个 60 秒切片
	RepliesPerRound int `json:"replies_per_round"` // 一轮可回应几次消息（评论/动态/私信共享）

	// DesireKeywords 本轮由对话者指令提炼的选片关键词：覆写 PreferenceKeywords，
	// 只在本轮生效，不写入配置文件；无欲求时才回退到配置默认偏好。
	DesireKeywords []string `json:"-"`
}

func defaultConfig() Config {
	return Config{
		DailyLimit:         defaultDailyWatchLimit,
		VideosPerRound:     defaultVideosPerRound,
		RepliesPerRound:    defaultRepliesPerRound,
		SlicesPerVideo:     defaultSlicesPerVideo,
		PreferenceKeywords: []string{"AI", "人工智能", "Qwen", "DeepSeek", "GLM", "Kimi", "Gemma", "肥鱼", "本地部署"},
	}
}

// clampInt 把取值夹紧到 [lo, hi]
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// mentionTargets 返回本轮分享要 @ 的全部目标：按配置顺序去重，最多 mentionMaxCount 人。
//
// 保留 UID（评论接口的 at_mid 需要它才能构建真实 @ 关系）。
// 开源项目不内置任何 @ 对象：未配置 mention_targets 时返回空列表，
// 调用方据此跳过 @ 分享，避免别人部署后 @ 到作者账号。
func (c Config) mentionTargets() []MentionTarget {
	targets := make([]MentionTarget, 0, mentionMaxCount)
	seen := make(map[string]bool, len(c.MentionTargets))
	for _, target := range c.MentionTargets {
		name := strings.TrimSpace(target.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		targets = append(targets, MentionTarget{Name: name, UID: target.UID})
		if len(targets) >= mentionMaxCount {
			break
		}
	}
	return targets
}

// mentionNames 返回本轮分享要 @ 的全部用户名（日志与 @分享评论文案使用）。
func (c Config) mentionNames() []string {
	targets := c.mentionTargets()
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}
	return names
}

func configPath() string {
	dir := strings.TrimSpace(config.LocalDir)
	if dir == "" {
		dir = "local_data"
	}
	return filepath.Join(dir, "bilibili_life.json")
}

// LoadConfig 读取用户参考词/过滤词；文件不存在或损坏时用默认值。
func LoadConfig() Config {
	cfg := defaultConfig()
	path := configPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		logger.Info("BilibiliLife", "未找到 %s，使用默认选片偏好", path)
		return cfg
	}
	// Windows 记事本/PowerShell 常把 JSON 存成带 BOM 的 UTF-8；
	// 解码前先去掉 BOM，否则 json.Unmarshal 会失败并静默退回默认配置（@目标会丢）。
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var saved Config
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn("BilibiliLife", "解析 %s 失败，使用默认选片偏好: %v", path, err)
		return cfg
	}
	if len(saved.PreferenceKeywords) > 0 {
		cfg.PreferenceKeywords = saved.PreferenceKeywords
	}
	if len(saved.MentionTargets) > 0 {
		valid := make([]MentionTarget, 0, len(saved.MentionTargets))
		for _, target := range saved.MentionTargets {
			if strings.TrimSpace(target.Name) != "" {
				valid = append(valid, MentionTarget{Name: strings.TrimSpace(target.Name), UID: target.UID})
			}
		}
		if len(valid) > 0 {
			cfg.MentionTargets = valid
		}
	}
	if saved.DailyLimit > 0 {
		cfg.DailyLimit = saved.DailyLimit
	}
	if saved.VideosPerRound > 0 {
		cfg.VideosPerRound = clampInt(saved.VideosPerRound, videoMinCount, videoMaxCount)
	}
	if saved.RepliesPerRound > 0 {
		cfg.RepliesPerRound = saved.RepliesPerRound
	}
	if saved.SlicesPerVideo > 0 {
		cfg.SlicesPerVideo = saved.SlicesPerVideo
	}
	return cfg
}

func scoreOf(text string, keywords []string) int {
	lower := strings.ToLower(text)
	score := 0
	for _, keyword := range keywords {
		word := strings.ToLower(strings.TrimSpace(keyword))
		if word != "" && strings.Contains(lower, word) {
			score += 3
		}
	}
	return score
}

func formatKeywords(words []string) string {
	if len(words) == 0 {
		return "（空）"
	}
	return fmt.Sprintf("%v", words)
}
