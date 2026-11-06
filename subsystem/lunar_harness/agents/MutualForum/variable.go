package MutualForum

// variable.go Mutual Forum（互助论坛）智能体与桥接层的常量与全局变量。

import (
	"errors"
	"sync"
	"time"
)

// ==== 智能体侧常量与全局变量 ====

const (
	// roleLabel 该智能体在「推理实况」中的角色标签（模块名 Mutual Forum）
	roleLabel = "互助论坛"
	// maxToolRounds 单次需求允许的工具调用轮次上限
	maxToolRounds = 8
	// modelTimeout 单次模型调用超时
	modelTimeout = 90 * time.Second
	// maxPostPreview 工具结果里的发言摘要长度
	maxPostPreview = 1200
	// maxBodyPreview 工具结果里的话题正文摘要长度
	maxBodyPreview = 512
)

var (
	// lastRunMu 保护最近一次模型轮次的诊断信息
	lastRunMu sync.Mutex
	// lastRun 最近一次模型轮次的轮次、工具调用数与错误（供 agentdump 展示）
	lastRun map[string]any
)

// ==== 论坛桥接层常量与全局变量 ====

const (
	// configFileName 独立配置文件默认相对路径（位于 local_data/config 下）
	configFileName = "config/forum.json"
	// configEnvKey 测试或高级部署可用来覆盖配置文件路径
	configEnvKey = "MUTUAL_FORUM_CONFIG"

	// defaultAccountRelative 默认账号/轮次状态文件（相对 local_data）
	defaultAccountRelative = "database/forum/forum_account.json"

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

	// enrollNameMaxRunes 申领名长度上限（与文档 1–64 字符一致）。
	enrollNameMaxRunes = 64
	// topicTitleMaxRunes 话题标题上限（文档 1–120）。
	topicTitleMaxRunes = 120
	// topicBodyMaxRunes 话题/发言正文上限（文档 1–8000）。
	topicBodyMaxRunes = 8000
	// commentBodyMaxRunes 评论正文上限（文档 1–2000）。
	commentBodyMaxRunes = 2000
	// payloadMaxBytes 请求体上限（文档 64KB）。
	payloadMaxBytes = 64 << 10

	// unreadFetchLimit 单批拉取的未读回应事件上限。
	unreadFetchLimit = 50

	// defaultAutoReplyMaxRetries 自动回应提交失败的默认重试上限。
	defaultAutoReplyMaxRetries = 3
	// maxAutoReplyMaxRetries 自动回应重试上限的配置上限。
	maxAutoReplyMaxRetries = 10
	// maxReplyErrorRunes 待回应条目里保存的错误摘要长度。
	maxReplyErrorRunes = 300

	// autoReplyStateVersion 未读回应重试状态的磁盘格式版本。
	autoReplyStateVersion = 1
	// pendingStoreSuffix 未读回应重试状态文件的后缀（与账号文件同目录）。
	pendingStoreSuffix = ".pending.json"
)

// unreadReplyTypes 只处理真正需要回应的「回应类」事件：新评论、新发言、被 @、被引用。
// 点赞、投票等只读事件不触发自动回复。
var unreadReplyTypes = []string{"comment_created", "post_created", "mention", "quote"}

var (
	// ErrConfigMissing 配置文件不存在
	ErrConfigMissing = errors.New("互助论坛配置文件不存在")
	// ErrDisabled 配置里没有开启允许使用开关
	ErrDisabled = errors.New("互助论坛未启用")
	// ErrInvalidBaseURL 服务地址为空或不是合法的 http(s) 地址
	ErrInvalidBaseURL = errors.New("互助论坛服务地址无效")
	// ErrCredentialsLost 本地凭证缺失且服务端申领命中了既有账号，无法自动取回 api_key
	ErrCredentialsLost = errors.New("互助论坛凭证丢失，且服务端未下发新的 api_key")
	// ErrQQInvalid 配置里的 qq 不符合 5–12 位纯数字且首位非 0
	ErrQQInvalid = errors.New("互助论坛配置的 qq 格式不正确（需 5–12 位数字且首位不为 0）")
	// ErrForumPageUnavailable 论坛页面无法打开（配置无效、健康检查失败或论坛未开放）；
	// 与「页面可用但没有待回应」区分：后者不返回错误且 Summary.Pending == 0。
	ErrForumPageUnavailable = errors.New("互助论坛页面无法打开")

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
