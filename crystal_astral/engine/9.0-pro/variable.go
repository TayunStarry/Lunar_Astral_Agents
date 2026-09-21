package ltp9

// ==== 常量与变量集中区 ====

import (
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/GeneralConfig"
)

// ServiceName 模块标识（琉璃命名）。
const ServiceName = "CrystalLTP9"

// EngineBuild 引擎构建标记：每次改动 callFn/导出回传逻辑时递增，供运行实例自证版本。
const EngineBuild = "2026-09-19"

// LTP9Tag metadata.json 标识 LTP9 包的标签（识别插件的唯一依据）。
const LTP9Tag = "LTP9"

// DefaultMain 插件主脚本（打包后的单一 JS）。
const DefaultMain = "execute.js"

// DefaultConfigFile 插件配置文件。
const DefaultConfigFile = "config.yaml"

// DataDirName 插件运行时数据目录（写入隔离，哈希校验时跳过）。
const DataDirName = "data"

// KeyFileName 权限密钥文件（被代码文件哈希加密的 allow-* 清单）。
const KeyFileName = "permissions.key"

// reconcileInterval 包目录对账轮询间隔。
const reconcileInterval = 3 * time.Second

// priorityUnset 事件订阅器未设置优先级时的标记（排序时视作最大的优先级，排在所有设置者之后，按时间顺序）。
const priorityUnset = -1

// ProbeDataDirName 前端可视化测试探针的文件区目录名（local_data/data 下）。
const ProbeDataDirName = "ltp9_probe"

// ProbePluginID 虚拟探针插件标识（文件区隔离用，非真实加载插件）。
const ProbePluginID = "__probe__"

// ==== LTP9 权限全集（allow-*） ====

// AllowPermissionNames LTP9 支持的全部权限。
var AllowPermissionNames = []string{
	"allow-file",
	"allow-database",
	"allow-memory",
	"allow-network",
	"allow-call",
	"allow-agent",
	"allow-signal",
	"allow-certificate",
	"allow-send",
	"allow-socket",
}

// allowedSet 权限名 → bool 查询集。
var allowedSet = func() map[string]bool {
	set := map[string]bool{}
	for _, p := range AllowPermissionNames {
		set[p] = true
	}
	return set
}()

// ==== 配置与全局（宿主可改动的可移植配置） ====

// LocalDir LTP9 运行时数据根目录（宿主可在 Init 前覆盖）。
// default 为空时回退到琉璃 GeneralConfig.LocalDir（与琉璃基板保持一致）。
var LocalDir string

// DeveloperMode 开发模式：默认 false 启用 permissions.key 代码哈希校验；
// 宿主可 SetDeveloperMode(true) 跳过校验并授予全部 allow-* 权限。
var DeveloperMode = false

// InsecureTLS 是否跳过服务端 TLS 证书校验（engine 网络 fetch / http 用）。
var InsecureTLS = false

// Engine 引擎管理器全局实例（Init 时创建）。
var Engine *engine

// outboundMu 保护 Engine.outbound 与 agentInvoker 的并发读写。
var outboundMu sync.RWMutex

// agentInvoker 前端智能体调用实现（engine.agent 真实通道，由主机注入）。
var agentInvoker func(appID, instruction string) (string, error)

// sendMu 保护 sendInvoker 的并发读写。
var sendMu sync.RWMutex

// sendInvoker 发送通道（engine.send 真实实现，由主机注入）。
var sendInvoker func(pluginID, kind, target string, payload any) error

// wsMu 保护 wsServicer 的并发读写。
var wsMu sync.RWMutex

// wsServicer WebSocket 服务端传输（engine.ws 真实实现，由主机注入）。
var wsServicer *WsBridge

// platformMu 保护 platformResolver 的并发读写。
var platformMu sync.RWMutex

// platformResolver 平台能力解析器（engine.platform 真实实现，由主机注入）。
var platformResolver func(method string, args map[string]any) (any, error)

// StickerCollection LTP9 表情包记忆库集合名（image 型集合，与记忆库约定一致）。
const StickerCollection = "stickers"

// ==== 模型垄断：engine.llm 单一虚拟模型池 ====
// 模型垄断：无论插件传什么参数（baseUrl/apiKey/model/taskType/selectionStrategy/model_list/
// ltp9_models.json 等），实际请求与返回配置一律以琉璃 agent 字段模型为准。
// 对话/多模态 → GeneralConfig.AgentMultimodal{Model,URL,Key}；
// 嵌入 → GeneralConfig.AgentEmbedding{Model,URL,Key}。

// modelInvokerMu / embedInvokerMu 保护对应注入函数（延迟注入：运行期后任意时刻可设）。
var modelInvokerMu sync.RWMutex
var modelInvoker func(messages []any, opts map[string]any) (any, error)

var embedInvokerMu sync.RWMutex
var embedInvoker func(input any, opts map[string]any) (any, error)

// creatorProviderName 模型垄断池的唯一 provider 标识（记账/熔断/查询用）。
const creatorProviderName = "crystal-agent"

// monoTasks LTP9 支持的任务类型（模型垄断下全部共享同一 agent 对话模型）。
var monoTasks = []string{"replyer", "planner", "tool_use", "vlm", "voice", "embedding"}

// modelUsageMu 保护模型调用次数（占位保留，模型垄断下恒为单候选）。
var modelUsageMu sync.Mutex
var modelUsageCounts = map[string]int{}

// globalModelStats 全局 LLM 调用记录收集器（engine.llm 调用记录）。
var globalModelStats = &ltmStatsCollector{records: make([]ltmCallRecord, 0, 256), maxSize: 10000}

// ==== 包根目录 ====

// rootOverride 宿主显式指定的 LTP9 包根目录；非空时优先于 LocalDir/package。
var rootOverride string

// localBase 返回 LTP9 运行时数据根目录（宿主 LocalDir 优先，否则回退琉璃 GeneralConfig.LocalDir）。
func localBase() string {
	if LocalDir != "" {
		return LocalDir
	}
	return *GeneralConfig.LocalDir
}

// packageRoot 计算 LTP9 包根目录。
// 默认扫描 可执行目录/{LocalDir|GeneralConfig.LocalDir}/package/，按 metadata.json 的 LTP9 标签筛选插件；
// 宿主可用 SetRootOverride 指定显式根目录。
func packageRoot() string {
	if rootOverride != "" {
		return rootOverride
	}
	return filepath.Join(localBase(), "package")
}