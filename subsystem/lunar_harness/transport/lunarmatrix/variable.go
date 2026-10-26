package lunarmatrix

// variable.go Lunar Matrix 桥接层的常量与全局变量。

import (
	"errors"
	"sync"
	"time"
)

const (
	// configFileName 独立配置文件默认文件名（位于 local_data 下）
	configFileName = "lunar_matrix.json"
	// configEnvKey 测试或高级部署可用来覆盖配置文件路径
	configEnvKey = "LUNAR_MATRIX_CONFIG"

	// defaultAccountRelative 默认账号/轮次状态文件（相对 local_data）
	defaultAccountRelative = "forum/lunar_matrix_account.json"

	defaultRequestTimeout  = 25
	defaultHealthTimeout   = 3
	defaultHealthCache     = 30
	defaultPollMinInterval = 5
	defaultRoundTTL        = 600
	defaultSearchLimit     = 10
	maxSearchLimit         = 50

	// accountValidationTTL 已有凭证的复验间隔：避免每次工具调用都请求 /me
	accountValidationTTL = 10 * time.Minute

	// roundRefreshGuard 轮次临近过期时提前续领的余量（秒）
	roundRefreshGuard = 30

	// maxPollRetries 429 poll_too_frequent 时的自动重试次数
	maxPollRetries = 2

	// apiPrefix 论坛 API 前缀
	apiPrefix = "/api/forum/v1"

	// clientVersion 默认客户端版本
	clientVersion = "lunar-matrix/1.0.0"
)

var (
	// ErrConfigMissing 配置文件不存在
	ErrConfigMissing = errors.New("互助论坛配置文件不存在")
	// ErrDisabled 配置里没有开启允许使用开关
	ErrDisabled = errors.New("互助论坛未启用")
	// ErrInvalidBaseURL 服务地址为空或不是合法的 http(s) 地址
	ErrInvalidBaseURL = errors.New("互助论坛服务地址无效")

	// configMu 保护配置缓存
	configMu sync.Mutex
	// configCache 最近一次解析成功的配置
	configCache Config
	// configCachePath 配置缓存对应的文件路径
	configCachePath string
	// configCacheMod 配置缓存对应的文件修改时间
	configCacheMod time.Time
	// configCacheSize 配置缓存对应的文件大小
	configCacheSize int64

	// probeMu 保护就绪探测缓存
	probeMu sync.Mutex
	// probeCache 最近一次健康探测结果
	probeCache readinessProbe
	// probeRunning 是否已有探测在飞行中（避免并发重复请求）
	probeRunning bool
)
