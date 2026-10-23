package BilibiliLife

// 生活链 · B 站真实浏览器版：选片偏好与安全过滤。
//
// 用户参考词与过滤词放在 local_data/bilibili_life.json（已 gitignore），
// 不在代码里硬编码个人偏好；文件缺失时使用安全默认值。

import (
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
	// dynamicSummaryMaxLines 动态正文里的简述最多 10 行
	dynamicSummaryMaxLines = 10
)

type Config struct {
	// 更倾向看的主题词（标题/卡片文本命中即加分）
	PreferenceKeywords []string `json:"preference_keywords"`
	// @分享评论可以 @ 的用户列表；不再硬编码为某一个人
	MentionTargets []MentionTarget `json:"mention_targets"`
	// 直接过滤：涉政、对立、录播/付费等
	BlockedKeywords []string `json:"blocked_keywords"`

	// 自动生活（TUI 强制触发不占每日次数）
	AutoEnabled     *bool   `json:"auto_enabled"`
	IntervalHours   float64 `json:"interval_hours"`
	DailyLimit      int     `json:"daily_limit"`
	VideosPerRound  int     `json:"videos_per_round"`  // 一轮最多刷几个视频（1-5）
	SlicesPerVideo  int     `json:"slices_per_video"`  // 每个视频理解几个 60 秒切片
	RepliesPerRound int     `json:"replies_per_round"` // 一轮可回应几次消息（评论/动态/私信共享）

	// OnlyCallUserInDynamic 仅在动态中呼叫用户（默认开启）：
	// 开启时不再从本轮多个视频里挑一条发「@ 用户」的分享评论，而是只在日记动态末尾
	// 另起一行 @ 目标用户；关闭时恢复原有「挑视频 + 发 @分享评论」逻辑。
	OnlyCallUserInDynamic *bool `json:"only_call_user_in_dynamic"`
}

func defaultConfig() Config {
	return Config{
		AutoEnabled:        boolPtr(true),
		IntervalHours:      6,
		DailyLimit:         2,
		VideosPerRound:        defaultVideosPerRound,
		RepliesPerRound:       defaultRepliesPerRound,
		SlicesPerVideo:        defaultSlicesPerVideo,
		OnlyCallUserInDynamic: boolPtr(true),
		PreferenceKeywords:    []string{"AI", "人工智能", "Qwen", "DeepSeek", "小肥鱼", "大肥鱼", "AI娘"},
		BlockedKeywords: []string{
			// 旧版基础过滤
			"直播回放", "录播", "付费", "充电专属",
			// 涉政
			"政治", "时政", "涉政", "意识形态", "中共", "共产党", "政府", "主席", "总统",
			"选举", "两会", "人大", "政协", "外交", "中美", "俄乌", "乌克兰", "俄罗斯",
			"巴以", "以色列", "巴勒斯坦", "台海", "台湾", "香港", "新疆", "西藏",
			"南海", "钓鱼岛", "领土", "战争", "制裁", "军事", "军队", "政变",
			"抗议", "游行", "示威", "民主", "专制", "独裁", "社会主义", "资本主义",
			"左派", "右派", "粉红", "公知", "殖人", "润人", "汉奸", "卖国",
			"爱国", "辱华", "抵制", "封杀",
			// 对立/引战
			"男女对立", "性别对立", "女权", "男权", "打拳", "拳师", "彩礼", "婚恋",
			"婆媳", "阶级", "贫富", "资本家", "打工人", "内卷", "躺平", "对立",
			"引战", "对线", "挂人", "网暴", "撕逼", "骂战", "饭圈", "粉丝互撕",
			"地域黑", "地图炮", "种族", "民族",
		},
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

func boolPtr(v bool) *bool { return &v }

// mentionNames 返回本轮分享要 @ 的全部目标：按配置顺序去重，最多 mentionMaxCount 人。
//
// 开源项目不内置任何 @ 对象：未配置 mention_targets 时返回空列表，
// 调用方据此跳过 @ 分享，避免别人部署后 @ 到作者账号。
func (c Config) mentionNames() []string {
	names := make([]string, 0, mentionMaxCount)
	seen := make(map[string]bool, len(c.MentionTargets))
	for _, target := range c.MentionTargets {
		name := strings.TrimSpace(target.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
		if len(names) >= mentionMaxCount {
			break
		}
	}
	return names
}

func (c Config) autoEnabled() bool {
	return c.AutoEnabled == nil || *c.AutoEnabled
}

// onlyCallUserInDynamic 未显式配置时按开启处理（默认行为）。
func (c Config) onlyCallUserInDynamic() bool {
	return c.OnlyCallUserInDynamic == nil || *c.OnlyCallUserInDynamic
}

func configPath() string {
	dir := strings.TrimSpace(*config.LocalDir)
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
	if saved.AutoEnabled != nil {
		cfg.AutoEnabled = saved.AutoEnabled
	}
	if saved.OnlyCallUserInDynamic != nil {
		cfg.OnlyCallUserInDynamic = saved.OnlyCallUserInDynamic
	}
	if saved.IntervalHours > 0 {
		cfg.IntervalHours = saved.IntervalHours
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
	if len(saved.BlockedKeywords) > 0 {
		// 用户过滤词与内置安全词合并；内置涉政/对立词不能因为文件漏写而消失
		seen := map[string]bool{}
		merged := make([]string, 0, len(cfg.BlockedKeywords)+len(saved.BlockedKeywords))
		for _, word := range append(cfg.BlockedKeywords, saved.BlockedKeywords...) {
			word = strings.TrimSpace(word)
			if word == "" || seen[word] {
				continue
			}
			seen[word] = true
			merged = append(merged, word)
		}
		cfg.BlockedKeywords = merged
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

func blockedBy(text string, keywords []string) (string, bool) {
	lower := strings.ToLower(text)
	for _, keyword := range keywords {
		word := strings.TrimSpace(keyword)
		if word != "" && strings.Contains(lower, strings.ToLower(word)) {
			return word, true
		}
	}
	return "", false
}

func formatKeywords(words []string) string {
	if len(words) == 0 {
		return "（空）"
	}
	return fmt.Sprintf("%v", words)
}
