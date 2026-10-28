package MutualForum

// variable.go Mutual Forum（互助论坛）智能体与桥接层的常量与全局变量。

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

// ==== 智能体侧常量与全局变量 ====

const (
	// roleLabel 该智能体在「核心智能 实况」中的角色标签（模块名 Mutual Forum）
	roleLabel = "互助论坛"
	// defaultModelName 未配置多模态模型时的兜底名称
	defaultModelName = "primary_reasoning"
	// maxToolRounds 单次需求允许的工具调用轮次上限
	maxToolRounds = 8
	// historyLimitMessages 独立上下文保留的消息条数（约 20 轮问答）
	historyLimitMessages = 40
	// runTimeout 单次需求的整体超时
	runTimeout = 2 * time.Minute
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
	// maxPostPreview 工具结果里的发言摘要长度
	maxPostPreview = 1200
	// maxBodyPreview 工具结果里的话题正文摘要长度
	maxBodyPreview = 240
)

var (
	// runMu 串行化自然语言需求，避免同一身份并发发言导致轮次互相覆盖
	runMu sync.Mutex

	// httpClient 模型调用客户端
	httpClient = &http.Client{Timeout: modelTimeout}

	// 模型端点只解析一次
	modelOnce sync.Once
	modelName string
	modelURL  string
	modelKey  string

	// historyMu 保护独立上下文
	historyMu sync.Mutex
	// history 最近若干轮「用户需求 + 最终答复」
	history []chatMessage

	// observerMu 保护实况观察者
	observerMu sync.Mutex
	obsBegin   func(role string)
	obsDelta   func(role, kind, text string)
	obsEnd     func(role string)
	obsMeter   func(prompt, completion, cached int64, generation time.Duration, estimated bool)
)

// ==== 论坛桥接层常量与全局变量 ====

const (
	// configFileName 独立配置文件默认文件名（位于 local_data 下）
	configFileName = "mutual_forum.json"
	// configEnvKey 测试或高级部署可用来覆盖配置文件路径
	configEnvKey = "MUTUAL_FORUM_CONFIG"

	// defaultAccountRelative 默认账号/轮次状态文件（相对 local_data）
	defaultAccountRelative = "forum/mutual_forum_account.json"

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
	clientVersion = "mutual-forum/1.0.0"
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
