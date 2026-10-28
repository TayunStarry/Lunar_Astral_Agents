package BilibiliViewer

// config.go 哔哩观影的配置读取：选片偏好与数量类参数。
//
// 用户参考词放在 local_data/bilibili_life.json（已 gitignore），
// 不在代码里硬编码个人偏好；文件缺失时使用安全默认值。
//
// @ 目标不再来自配置：分享时从当前账号的「特别关注」列表里随机挑选。

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

// ==== 一轮生活的固定机制参数 ====
//
// 以下取值是「机制」而非「偏好」，因此不做成配置项：
// 每个视频读固定条数的热门评论、固定只写 1 条评论。
// 观看切片数由审核模型按视频时长决定（2-8），不再有配置项。
const (
	// defaultRepliesPerRound 默认一轮回应 5 次被动消息（@ 与追评共用这一份额度）
	defaultRepliesPerRound = 5
)

func defaultConfig() Config {
	return Config{
		RepliesPerRound:    defaultRepliesPerRound,
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

func configPath() string {
	dir := strings.TrimSpace(config.LocalDir)
	if dir == "" {
		dir = "local_data"
	}
	return filepath.Join(dir, "bilibili_life.json")
}

// LoadConfig 读取用户参考词与数量参数；文件不存在或损坏时用默认值。
func LoadConfig() Config {
	cfg := defaultConfig()
	path := configPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		logger.Info("BilibiliViewer", "未找到 %s，使用默认选片偏好", path)
		return cfg
	}
	// Windows 记事本/PowerShell 常把 JSON 存成带 BOM 的 UTF-8；
	// 解码前先去掉 BOM，否则 json.Unmarshal 会失败并静默退回默认配置。
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var saved Config
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn("BilibiliViewer", "解析 %s 失败，使用默认选片偏好: %v", path, err)
		return cfg
	}
	if len(saved.PreferenceKeywords) > 0 {
		cfg.PreferenceKeywords = saved.PreferenceKeywords
	}
	if saved.RepliesPerRound > 0 {
		cfg.RepliesPerRound = saved.RepliesPerRound
	}
	return cfg
}

// scoreOf 文本命中关键词的数量得分（标题/标签/评论摘要通用）
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
