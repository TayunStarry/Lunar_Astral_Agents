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

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

type Config struct {
	// 更倾向看的主题词（标题/卡片文本命中即加分）
	PreferenceKeywords []string `json:"preference_keywords"`
	// 直接过滤：涉政、对立、录播/付费等
	BlockedKeywords []string `json:"blocked_keywords"`
}

func defaultConfig() Config {
	return Config{
		PreferenceKeywords: []string{"AI", "人工智能", "Qwen", "DeepSeek", "小肥鱼", "大肥鱼", "AI娘"},
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

func configPath() string {
	dir := strings.TrimSpace(*GeneralConfig.LocalDir)
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
		LoggerGeneral.Info("BilibiliLife", "未找到 %s，使用默认选片偏好", path)
		return cfg
	}
	var saved Config
	if err := json.Unmarshal(raw, &saved); err != nil {
		LoggerGeneral.Warn("BilibiliLife", "解析 %s 失败，使用默认选片偏好: %v", path, err)
		return cfg
	}
	if len(saved.PreferenceKeywords) > 0 {
		cfg.PreferenceKeywords = saved.PreferenceKeywords
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
	LoggerGeneral.Info("BilibiliLife", "选片偏好：参考词 %d 个；过滤词 %d 个（%s）",
		len(cfg.PreferenceKeywords), len(cfg.BlockedKeywords), path)
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
