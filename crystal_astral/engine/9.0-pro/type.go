package YaraLTP

import (
	"encoding/json"
	"sync"

	"github.com/dop251/goja"
)

// ==== 消息体 ====

// YaraMessage 聊天消息对象（事件 / Hook 回调参数，对齐 yara.d.ts）。
type YaraMessage struct {
	ID         string   `json:"id"`
	SenderID   string   `json:"senderId"`
	SenderName string   `json:"senderName"`
	GroupID    string   `json:"groupId"`
	Content    string   `json:"content"`
	IsAtMe     bool     `json:"isAtMe,omitempty"`
	HasImage   bool     `json:"hasImage,omitempty"`
	ImageURLs  []string `json:"image_urls,omitempty"`
	Timestamp  int64    `json:"timestamp"`
	Platform   string   `json:"platform"`
}

// ==== 订阅与注册条目 ====

// jsFunc goja 回调引用（在插件 VM 内调用）。
type jsFunc = goja.Value

// hookSub 一个插件内注册的 Hook 订阅项。
type hookSub struct {
	hookType   string
	mode       string // blocking / observe
	order      string // early / normal / late
	errorPolicy string // abort / skip / log
	timeoutMs  int64
	handler    jsFunc
}

// eventSub 事件订阅项（event.subscribe 与 eventHandler.register 共用）。
type eventSub struct {
	name   string
	weight int
	interceptMessage bool
	handler jsFunc
}

// commandDef 指令定义。
type commandDef struct {
	name    string
	pattern string
	handler jsFunc
	aliases []string
}

// toolDef 工具定义。
type toolDef struct {
	name             string
	description      string
	briefDescription string
	detailedDescription string
	visibility       string // visible / hidden / deferred
	toolType         string // agent / autonomous / core
	timeoutSeconds   int64
	async            bool
	hookType         string
	pattern          string
	parameters       []toolParam
	handler          jsFunc
}

// toolParam 工具参数。
type toolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`
	EnumValues  []string `json:"enumValues,omitempty"`
}

// apiDef 插件暴露的跨插件 API（yara.api.register）。
type apiDef struct {
	name        string
	description string
	version     string
	public      bool
	handler     jsFunc
}

// llmProvider 插件自定义 LLM 提供商。
type llmProvider struct {
	name    string
	clientType string
	handler jsFunc
}

// ==== 插件（每插件独立 goja 沙箱） ====

// plugin 一个 LTP3 包对应的运行实例。
type plugin struct {
	ID         string // metadata.id
	DirName    string // 包目录名
	Title      string // metadata.title
	Root       string // 包目录绝对路径
	MainPath   string // index.js 绝对路径
	ConfigPath string // config.yaml 绝对路径
	DataDir    string // data/ 绝对路径
	KeyPath    string // permissions.key 绝对路径

	config map[string]any // 解析后的 config.yaml 内容
	granted map[string]bool // 本次加载经脚本哈希校验通过的权限集合

	vm  *goja.Runtime
	mu  sync.Mutex // 串行化同一插件所有 JS 执行（goja 非线程安全）

	loaded   bool
	loadErr  string
	onLoadFn jsFunc
	onUnloadFn jsFunc
	onConfigUpdateFn jsFunc

	hooks        map[string][]*hookSub
	events       map[string][]*eventSub
	commands     map[string]*commandDef
	tools        map[string]*toolDef
	toolRegOrder []string // 记录工具注册顺序
	apis         map[string]*apiDef
	llmProviders map[string]*llmProvider

	// 执行上下文：当前是否为一次 hook/event 分发触发的（用于 send 单播回执）
	currentRequestID string
}

// ==== 引擎管理器 ====

// engine LTP3 引擎管理器：负责包扫描、虚拟机加载/卸载、事件与钩子分发。
type engine struct {
	mu       sync.RWMutex
	plugins  map[string]*plugin // ID → 插件
	byDir    map[string]string  // 包目录名 → ID
	root     string             // 包根目录
	running  bool
	busFingerprint string // 上次对账快照（目录名:校验，用于探测增删）
}

// ==== Hook / 事件分发结果 ====

// hookOutcome 单个插件的钩子执行结果。
type hookOutcome struct {
	PluginID string `json:"plugin_id"`
	Error    string `json:"error,omitempty"`
	Handled  bool   `json:"handled"`
	Result   any    `json:"result,omitempty"` // JS 返回对象（allowContinue/action/modifiedData...）
}

// dispatchSummary 聚合后的分发汇总。
type dispatchSummary struct {
	Subscribed   int `json:"subscribed"`
	Errored      int `json:"errored"`
	AllowContinue bool `json:"allow_continue"`
	Aborted      bool `json:"aborted"`
}

// ==== WS 总线信封（engine ↔ 真实客户端） ====

// InMessage 客户端 → 引擎 的请求信封。
type InMessage struct {
	Type      string          `json:"type"` // ltp3/hook | ltp3/event | ltp3/command | ltp3/tool | ltp3/manage | ltp3/ping
	RequestID string          `json:"request_id,omitempty"`
	Hook      string          `json:"hook,omitempty"`
	Event     string          `json:"event,omitempty"`
	Command   string          `json:"command,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Action    string          `json:"action,omitempty"` // manage: list|scan|reload|reload_one|unload_one
	ID        string          `json:"id,omitempty"`     // manage 目标插件 ID
	Match     []string        `json:"match,omitempty"`  // command 正则匹配组
	Context   map[string]any  `json:"context,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// hookResultMessage 引擎 → 客户端：钩子分发结果。
type hookResultMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Hook      string          `json:"hook"`
	Results   []hookOutcome   `json:"results"`
	Summary   dispatchSummary `json:"summary"`
}

// eventAckMessage 引擎 → 客户端：事件发布确认。
type eventAckMessage struct {
	Type       string `json:"type"`
	RequestID  string `json:"request_id,omitempty"`
	Event      string `json:"event"`
	Subscribed int    `json:"subscribed"`
}

// commandResultMessage 引擎 → 客户端：指令调用结果。
type commandResultMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Command   string          `json:"command"`
	Results   []hookOutcome   `json:"results"`
	Summary   dispatchSummary `json:"summary"`
}

// toolResultMessage 引擎 → 客户端：工具调用结果。
type toolResultMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Tool      string          `json:"tool"`
	Results   []hookOutcome   `json:"results"`
	Summary   dispatchSummary `json:"summary"`
}

// managePayload 管理动作返回的插件状态列表。
type manageState struct {
	ID      string `json:"id"`
	DirName string `json:"dir_name"`
	Title   string `json:"title"`
	Loaded  bool   `json:"loaded"`
	Error   string `json:"error,omitempty"`
}

// manageAckMessage 引擎 → 客户端：管理动作确认。
type manageAckMessage struct {
	Type      string        `json:"type"`
	RequestID string        `json:"request_id,omitempty"`
	Action    string        `json:"action"`
	OK        bool          `json:"ok"`
	Message   string        `json:"message,omitempty"`
	Plugins   []manageState `json:"plugins,omitempty"`
}

// pongMessage 引擎 → 客户端：存活确认。
type pongMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Engine    string `json:"engine"`
	Plugins   int    `json:"plugins"`
}

// outMessage 引擎 → 客户端：通用结果/错误。
type outMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// sendMessage 引擎 → 客户端：插件产生的消息发送请求。
type sendMessage struct {
	Type       string `json:"type"`
	RequestID  string `json:"request_id,omitempty"` // 有值 → 单播回触发客户端；无值 → 默认广播
	PluginID   string `json:"plugin_id,omitempty"`
	Kind       string `json:"kind"` // text | image | emoji | hybrid
	GroupID    string `json:"group_id,omitempty"`
	Content    string `json:"content,omitempty"`
	Image      string `json:"image,omitempty"`
	Emoji      string `json:"emoji,omitempty"`
	Segments   []any  `json:"segments,omitempty"`
	Success    bool   `json:"success"`
}

// lifecycleMessage 引擎 → 客户端：插件加载 / 卸载生命周期广播。
type lifecycleMessage struct {
	Type   string `json:"type"`
	Event  string `json:"event"` // ON_START / ON_STOP
	Plugin string `json:"plugin,omitempty"`
	Title  string `json:"title,omitempty"`
}

// ==== 工具/模型调用载荷（yara.model / yara.tool 用） ====

// toolCallArg 工具调用入参结构（api.tool 用 map）。
// chatMessage 等模型载荷类型见 model.go。