package lunarmatrix

// config.go 独立配置文件的读取、缓存与校验。
//
// 配置文件默认是 local_data/lunar_matrix.json；测试或特殊部署可通过
// 环境变量 LUNAR_MATRIX_CONFIG 指向别处。
//
// 这里刻意不依赖 engine/config 的统一配置：论坛的开关、地址、加入码与凭证
// 属于一个独立功能域，放在单独文件里，未配置有效地址时整个工具不暴露。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	engineconfig "LunarSubsystem/LunarHarness/engine/config"
)

// DefaultConfig 返回未配置时的兜底值（默认关闭）。
func DefaultConfig() Config {
	return Config{
		Enabled:                false,
		InstanceName:           "月华",
		ClientVersion:          clientVersion,
		RequestTimeoutSeconds:  defaultRequestTimeout,
		HealthTimeoutSeconds:   defaultHealthTimeout,
		HealthCacheSeconds:     defaultHealthCache,
		PollMinIntervalSeconds: defaultPollMinInterval,
		RoundTTLSeconds:        defaultRoundTTL,
		SearchLimit:            defaultSearchLimit,
	}
}

// ConfigPath 返回实际使用的配置文件路径。
func ConfigPath() string {
	if custom := strings.TrimSpace(os.Getenv(configEnvKey)); custom != "" {
		return custom
	}
	return filepath.Join(engineconfig.LocalDir, configFileName)
}

// LoadConfig 读取并缓存配置文件；文件修改后自动重新解析。
func LoadConfig() (Config, error) {
	path := ConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), fmt.Errorf("%w：%s", ErrConfigMissing, path)
		}
		return DefaultConfig(), fmt.Errorf("读取互助论坛配置失败: %w", err)
	}

	configMu.Lock()
	defer configMu.Unlock()
	if configCachePath == path && configCacheMod.Equal(info.ModTime()) && configCacheSize == info.Size() {
		return configCache, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig(), fmt.Errorf("读取互助论坛配置失败: %w", err)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	cfg := DefaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("解析互助论坛配置失败: %w", err)
	}
	cfg = cfg.withDefaults()

	configCache = cfg
	configCachePath = path
	configCacheMod = info.ModTime()
	configCacheSize = info.Size()
	return cfg, nil
}

// withDefaults 给未填写的数值/文本补上可用默认值。
func (c Config) withDefaults() Config {
	if strings.TrimSpace(c.InstanceName) == "" {
		c.InstanceName = "月华"
	}
	if strings.TrimSpace(c.ClientVersion) == "" {
		c.ClientVersion = clientVersion
	}
	if c.RequestTimeoutSeconds <= 0 {
		c.RequestTimeoutSeconds = defaultRequestTimeout
	}
	if c.RequestTimeoutSeconds > 180 {
		c.RequestTimeoutSeconds = 180
	}
	if c.HealthTimeoutSeconds <= 0 {
		c.HealthTimeoutSeconds = defaultHealthTimeout
	}
	if c.HealthTimeoutSeconds > 30 {
		c.HealthTimeoutSeconds = 30
	}
	if c.HealthCacheSeconds <= 0 {
		c.HealthCacheSeconds = defaultHealthCache
	}
	if c.HealthCacheSeconds > 600 {
		c.HealthCacheSeconds = 600
	}
	if c.PollMinIntervalSeconds <= 0 {
		c.PollMinIntervalSeconds = defaultPollMinInterval
	}
	if c.RoundTTLSeconds <= 0 {
		c.RoundTTLSeconds = defaultRoundTTL
	}
	if c.SearchLimit <= 0 {
		c.SearchLimit = defaultSearchLimit
	}
	if c.SearchLimit > maxSearchLimit {
		c.SearchLimit = maxSearchLimit
	}
	return c
}

// Validate 校验开关与地址；未通过时工具必须保持隐藏。
func (c Config) Validate() error {
	if !c.Enabled {
		return ErrDisabled
	}
	if _, err := url.ParseRequestURI(c.Endpoint()); err != nil || c.Endpoint() == "" {
		return ErrInvalidBaseURL
	}
	parsed, err := url.Parse(c.Endpoint())
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ErrInvalidBaseURL
	}
	return nil
}

// Endpoint 返回规范化后的 API 根地址；地址非法时返回空串。
func (c Config) Endpoint() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return ""
	}
	if !strings.HasSuffix(base, apiPrefix) {
		base += apiPrefix
	}
	return base
}

// SiteURL 返回站点根地址（用于拼接人类只读页面链接）。
func (c Config) SiteURL() string {
	endpoint := c.Endpoint()
	if endpoint == "" {
		return ""
	}
	return strings.TrimSuffix(endpoint, apiPrefix)
}

// AccountPath 返回账号/轮次状态文件路径。
func (c Config) AccountPath() string {
	if path := strings.TrimSpace(c.AccountFile); path != "" {
		return path
	}
	return filepath.Join(engineconfig.LocalDir, filepath.FromSlash(defaultAccountRelative))
}

// RequestTimeout 单次 API 请求超时。
func (c Config) RequestTimeout() time.Duration {
	return time.Duration(c.RequestTimeoutSeconds) * time.Second
}

// HealthTimeout 就绪探测超时。
func (c Config) HealthTimeout() time.Duration {
	return time.Duration(c.HealthTimeoutSeconds) * time.Second
}

// HealthCache 就绪探测缓存时长。
func (c Config) HealthCache() time.Duration {
	return time.Duration(c.HealthCacheSeconds) * time.Second
}

// PollMinInterval 兜底最小轮询间隔（秒）。
func (c Config) PollMinInterval() int {
	if c.PollMinIntervalSeconds <= 0 {
		return defaultPollMinInterval
	}
	return c.PollMinIntervalSeconds
}

// RoundTTL 兜底轮次有效期（秒）。
func (c Config) RoundTTL() int {
	if c.RoundTTLSeconds <= 0 {
		return defaultRoundTTL
	}
	return c.RoundTTLSeconds
}

// SearchSize 本地搜索默认条数。
func (c Config) SearchSize() int {
	if c.SearchLimit <= 0 {
		return defaultSearchLimit
	}
	if c.SearchLimit > maxSearchLimit {
		return maxSearchLimit
	}
	return c.SearchLimit
}
