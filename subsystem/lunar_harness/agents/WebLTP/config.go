package WebLTP

// Web-LTP · 网络搜索（web_search）内置智能体
// 琉璃内置的独立 LTPX AtoA 能力（同 AutoLTP 模式）：月华下发自然语言检索要求，
// 后端独立驱动真浏览器（browser.WebViewSession）完成：
//   搜索引擎降级链（bing → baidu → sogou，过滤字典站/工具站兜底并去重）
//   → 进入搜索结果页，从上到下完整滚动（触发懒加载、补全结果）
//   → 依次进入前 N（≤10）个结果页（跨结果页 URL 去重）
//     → 未过期缓存命中则直接复用 SQL 摘要（web.db 的 web_ltp_page_cache 表，默认 7 天，
//       超期记录照常访问并用新摘要覆写）
//     → 未命中则从页面顶部滚动到底部，提取正文并按段落分块摘要（文本为主）
//     → 只有正文少于 vision_min_chars 时才按需截取滚动画面，交给视觉模型补充信息
//     → 摘要硬切断后写缓存
//   → 回访搜索页（不滚动、不截图），做纯文本总结
//   → 以「网络搜索」过程口吻汇编搜索报告 → 回传月华。
// 不依赖任何前端包；业务完成后关闭浏览器页面；窗口中途被关闭则以已采集信息收尾。
// 配置沿用主配置文件 lunar_config.json 的 web_search 字段（缺失字段使用默认值）。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const webSearchPagesHardCap = 10

// LoadConfig 从 lunar_config.json 的 web_search 字段读取配置（每次运行重读，便于热更新）。
// 路径解析与 engine/config 对齐：LocalDir 为绝对路径时直接使用；
// 相对路径时依次尝试「可执行文件所在目录」「当前工作目录」。文件或字段缺失时用默认值。
func LoadConfig() Config {
	cfg := defaultConfig()

	path, err := resolveLunarConfigPath()
	if err != nil {
		logger.SubWarn("CrystalAstral", "WebLTP", "定位 lunar_config.json 失败，使用默认配置: %v", err)
		return cfg
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		logger.SubWarn("CrystalAstral", "WebLTP", "读取 %s 失败，使用默认配置: %v", path, err)
		return cfg
	}
	var root struct {
		WebSearch *Config `json:"web_search"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		logger.SubWarn("CrystalAstral", "WebLTP", "解析 %s 失败，使用默认配置: %v", path, err)
		return cfg
	}
	if root.WebSearch == nil {
		return cfg // 未配置 web_search 字段：全部默认
	}
	w := *root.WebSearch
	// 字段兜底
	if w.MaxPages <= 0 {
		w.MaxPages = 3
	}
	if w.MaxPages > webSearchPagesHardCap {
		w.MaxPages = webSearchPagesHardCap
	}
	if w.MaxResults <= 0 {
		w.MaxResults = 15
	}
	if w.ResultsScrollCaptures <= 0 {
		w.ResultsScrollCaptures = 3
	}
	if w.PageScrollCaptures <= 0 {
		w.PageScrollCaptures = 10
	}
	if w.SummaryMaxChars <= 0 || w.SummaryMaxChars > 4096 {
		w.SummaryMaxChars = 4096
	}
	if w.MaxRunSeconds <= 0 {
		w.MaxRunSeconds = 360
	}
	if w.CacheTTLDays <= 0 {
		w.CacheTTLDays = 7
	}
	if w.ScreenshotDir == "" {
		w.ScreenshotDir = "captures/web_search"
	}
	if w.VisionMinChars <= 0 {
		w.VisionMinChars = 240
	}
	if w.TextChunkChars < 800 {
		w.TextChunkChars = 5000
	}
	if w.TextChunkChars > 8000 {
		w.TextChunkChars = 8000
	}
	if w.TextMaxChars < 2000 {
		w.TextMaxChars = 24000
	}
	if w.TextMaxChars > 60000 {
		w.TextMaxChars = 60000
	}
	if w.TextMaxChunks <= 0 {
		w.TextMaxChunks = 4
	}
	if w.TextMaxChunks > 12 {
		w.TextMaxChunks = 12
	}
	if w.ScrollSettleMS < 100 {
		w.ScrollSettleMS = 350
	}
	if w.ScrollSettleMS > 2000 {
		w.ScrollSettleMS = 2000
	}
	if w.ScrollMaxSteps < 5 {
		w.ScrollMaxSteps = 40
	}
	if w.ScrollMaxSteps > 120 {
		w.ScrollMaxSteps = 120
	}
	return w
}

// cacheEnabled 缓存开关（未配置时默认开启）
func (c Config) cacheEnabled() bool {
	return c.CacheEnabled == nil || *c.CacheEnabled
}

// defaultConfig 默认配置
func defaultConfig() Config {
	return Config{
		SaveScreenshots:       false,
		ScreenshotDir:         "captures/web_search",
		MaxPages:              3,
		MaxResults:            15,
		ResultsScrollCaptures: 3,
		PageScrollCaptures:    10,
		SummaryMaxChars:       4096,
		MaxRunSeconds:         360,
		CacheTTLDays:          7,
		VisionMinChars:        240,
		TextChunkChars:        5000,
		TextMaxChars:          24000,
		TextMaxChunks:         4,
		ScrollSettleMS:        350,
		ScrollMaxSteps:        40,
	}
}

// resolveLunarConfigPath 定位 lunar_config.json（与 engine/config 的解析语义一致）
func resolveLunarConfigPath() (string, error) {
	const name = "lunar_config.json"
	if filepath.IsAbs(*config.LocalDir) {
		p := filepath.Join(*config.LocalDir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return p, fmt.Errorf("配置文件不存在: %s", p)
	}
	var candidates []string
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), *config.LocalDir, name))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, *config.LocalDir, name))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], fmt.Errorf("未找到配置文件（已尝试 %d 个候选路径）", len(candidates))
	}
	return "", fmt.Errorf("无法定位配置文件")
}

// localDirFlag 暴露 LocalDir 标志（截图目录基于本地数据根目录）
func localDirFlag() *string {
	return config.LocalDir
}
