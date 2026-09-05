package YaraLTP

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/GeneralConfig"
)

// ==== 常量 ====

// ServiceName 模块标识。
const ServiceName = "LTP3"

// defaultHookTopic 默认承接钩子点（yara_ltp 工具路由目标）。
const defaultHookTopic = "chat.receive.after_process"

// LTP3Tag metadata.json 中标识 LTP3 包的标签（识别插件的唯一依据）。
const LTP3Tag = "LTP3"

// DefaultMain 插件主逻辑文件名（忽略其它版本的 plugin.json 与其它配置格式）。
const DefaultMain = "index.js"

// DefaultConfigFile 插件唯一配置文件（LTP3 仅支持 config.yaml）。
const DefaultConfigFile = "config.yaml"

// DataDirName 插件运行时数据目录（热重载/清理不涉及；本实现为局部目录）。
const DataDirName = "data"

// KeyFileName 插件权限密钥文件名：每权限一条 32 字符密钥字符串以 '+' 连接成明文报文后，
// 用脚本哈希整体加密成一段密文写入该文件（分隔符与密钥边界在密文中不可见）。
const KeyFileName = "permissions.key"

// AllPermissionNames 引擎支持的权限名全集（密钥生成器下拉与引擎校验共用）。
var AllPermissionNames = []string{
	"event.subscribe", "event.publish",
	"hook.register",
	"command.register",
	"tool.register",
	"event_handler.register",
	"llm_provider.register",
	"api.register", "api.call",
	"send.text", "send.image", "send.emoji", "send.hybrid",
	"http.request",
	"network.tcp", "network.udp",
	"platform.command",
	"encoding.use", "time.use", "crypto.use",
	"model.access",
	"plugin.config.read", "plugin.config.write",
	"plugin.file.read", "plugin.file.write",
	"data.directory.read", "data.directory.write",
	"database.read",
	"knowledge.search",
	"async_task.execute",
	"emoji.access",
}

// routeHookPriority 权重默认值（eventHandler.weight）。
const defaultEventHandlerWeight = 100

// reconcileInterval 包目录对账轮询间隔（运行时增删包 → 加载/卸载虚拟机）。
const reconcileInterval = 3 * time.Second

// httpDefaultTimeout 插件 HTTP 请求默认超时（秒），与协议文档一致。
const httpDefaultTimeout = 120

// scriptExecBudget 单插件串行队列排队上限（防御性）。
const execQueueCap = 256

// ==== 全局实例 ====

// Engine 引擎管理器全局实例（Init 时创建）。
var Engine *engine

// sender 出站总线发送函数（由 crystal_astral 注入，真正广播到 /ws 客户端）。
var sendOut func([]byte)

// sendMu 保护 sendOut 的并发设置/读取。
var sendMu sync.RWMutex

// reconcileStop 停止对账循环的信号通道（Close 时关闭）。
var reconcileStop chan struct{}

// reconcileWG 对账循环退出同步。
var reconcileWG sync.WaitGroup

// ==== 模型配置缓存（从 lunar_config.json 读取，不硬编码模型名） ====

var (
	modelCfgOnce sync.Once
	chatModel    string
	chatURL      string
	chatKey      string
	embedModel   string
	embedURL     string
	embedKey     string
)

// httpClient 插件网络/模型请求共享客户端。
var httpClient = &http.Client{Timeout: 120 * time.Second}

// ==== YaraEvents 全局事件常量（注入每个插件沙箱，权威源：LTP3协议文档/yara.d.ts） ====

var YaraEvents = map[string]string{
	"ON_START":               "ON_START",
	"ON_STOP":                "ON_STOP",
	"ON_MESSAGE_PRE_PROCESS": "ON_MESSAGE_PRE_PROCESS",
	"ON_MESSAGE":             "ON_MESSAGE",
	"ON_PLAN":                "ON_PLAN",
	"POST_LLM":               "POST_LLM",
	"AFTER_LLM":              "AFTER_LLM",
	"POST_SEND_PRE_PROCESS":  "POST_SEND_PRE_PROCESS",
	"POST_SEND":              "POST_SEND",
	"AFTER_SEND":             "AFTER_SEND",
}

// ==== YaraHooks 全局钩子常量（注入每个插件沙箱，权威源：LTP3协议文档/yara.d.ts） ====

var YaraHooks = map[string]string{
	"CHAT_RECEIVE_BEFORE_PROCESS":            "chat.receive.before_process",
	"CHAT_RECEIVE_AFTER_PROCESS":             "chat.receive.after_process",
	"CHAT_COMMAND_BEFORE_EXECUTE":            "chat.command.before_execute",
	"CHAT_COMMAND_AFTER_EXECUTE":             "chat.command.after_execute",
	"EMOJI_CHAT_BEFORE_SELECT":               "emoji.chat.before_select",
	"EMOJI_CHAT_AFTER_SELECT":                "emoji.chat.after_select",
	"EMOJI_REGISTER_AFTER_BUILD_DESCRIPTION": "emoji.register.after_build_description",
	"EMOJI_REGISTER_AFTER_BUILD_EMOTION":     "emoji.register.after_build_emotion",
	"SEND_SERVICE_AFTER_BUILD_MESSAGE":       "send_service.after_build_message",
	"SEND_SERVICE_BEFORE_SEND":               "send_service.before_send",
	"SEND_SERVICE_AFTER_SEND":                "send_service.after_send",
	"CHAT_PLANNER_BEFORE_REQUEST":            "chat.planner.before_request",
	"CHAT_PLANNER_AFTER_RESPONSE":            "chat.planner.after_response",
	"CHAT_REPLYER_BEFORE_REQUEST":            "chat.replyer.before_request",
	"CHAT_REPLYER_BEFORE_MODEL_REQUEST":      "chat.replyer.before_model_request",
	"CHAT_REPLYER_AFTER_RESPONSE":            "chat.replyer.after_response",
	"JARGON_QUERY_BEFORE_SEARCH":             "jargon.query.before_search",
	"JARGON_QUERY_AFTER_SEARCH":              "jargon.query.after_search",
	"JARGON_EXTRACT_BEFORE_PERSIST":          "jargon.extract.before_persist",
	"JARGON_INFERENCE_BEFORE_FINALIZE":       "jargon.inference.before_finalize",
	"EXPRESSION_SELECT_BEFORE_SELECT":        "expression.select.before_select",
	"EXPRESSION_SELECT_AFTER_SELECTION":      "expression.select.after_selection",
	"EXPRESSION_LEARN_AFTER_EXTRACT":         "expression.learn.after_extract",
	"EXPRESSION_LEARN_BEFORE_UPSERT":         "expression.learn.before_upsert",
}

// baseDir 计算 LTP3 包根目录（可执行目录/local_data/package）。
// 与 crystal_astral 的 packageBaseDir 保持一致（独立实现避免跨包依赖）。
func packageRoot() string {
	execPath, err := os.Executable()
	if err != nil {
		return filepath.Join("local_data", "package")
	}
	execDir := filepath.Dir(execPath)
	return filepath.Join(execDir, *GeneralConfig.LocalDir, "package")
}
