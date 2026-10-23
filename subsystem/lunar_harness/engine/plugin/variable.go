package StarLTP

// ==== 常量与变量集中区 ====

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// ServiceName 模块标识。
const ServiceName = "StarLTP"

// EngineBuild 引擎构建标记：每次改动沙箱绑定/回调执行逻辑时递增，供运行实例自证版本。
const EngineBuild = "2026-10-23"

// ==== 月华事件主题（全部由 Go 侧边界触发，goja 不再发起事件） ====

const (
	// EventBeforeReadingMessage 读取消息前：改写即将交付给月华的消息
	EventBeforeReadingMessage = "before_reading_message"
	// EventBeforeSendingMessage 消息发送前：改写、追加或取消即将发出的消息
	EventBeforeSendingMessage = "before_sending_amessage"
	// EventBeforeQueryStarts 查询开始前：改写或取消网络搜索请求
	EventBeforeQueryStarts = "before_query_starts"
	// EventBeforeQueryEnds 查询结束前：改写或取消搜索报告
	EventBeforeQueryEnds = "before_query_ends"
	// EventBeforeLifeStarts 生活开始前：可取消本次生活（取消仍消耗一次本日额度并计为完成）
	EventBeforeLifeStarts = "before_life_starts"
	// EventBeforeLifeEnds 生活结束前：纯触发通知，返回值不被使用
	EventBeforeLifeEnds = "before_life_ends"
)

// LTP9Tag metadata.json 标识 LTP9 包的标签。
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

// AllPermissions 权限名 → bool 查询集。
var allowedSet = func() map[string]bool {
	set := map[string]bool{}
	for _, p := range AllowPermissionNames {
		set[p] = true
	}
	return set
}()

// ==== 配置与全局 ====

// Engine 引擎管理器全局实例（Init 时创建）。
var Engine *engine

// outboundMu 保护 Engine.outbound 与 agentInvoker 的并发读写。
var outboundMu sync.RWMutex

// agentInvoker 前端智能体调用实现（agent.synergy 真实通道，由宿主注入）。
var agentInvoker func(appID, instruction string) (string, error)

// ==== 蓝图执行器全局 ====

// bpDefaultMaxCalls 单次运行默认最大节点执行数量（对齐前端默认值，limit 节点可动态调整）。
const bpDefaultMaxCalls = 300

// bpNodeTimeout 单个引擎节点执行等待上限（对齐前端 sendWait 默认 5 分钟）。
const bpNodeTimeout = 5 * time.Minute

// bpExecTimeout 浏览器协作节点（microphone/speaker/audio_eq 等）执行回报等待上限。
const bpExecTimeout = 5 * time.Minute

// bpMu 保护 bpCurrent（单运行实例约束：同一时刻最多一个蓝图运行）。
var bpMu sync.Mutex

// bpCurrent 当前运行的蓝图执行器实例；nil 表示空闲。
var bpCurrent *bpRunner

// ==== 蓝图事件观察者（event_start 的事件来源：Emit / 插件 event.publish） ====

// emitObsMu 保护 emitObserver。
var emitObsMu sync.RWMutex

// emitObserver 蓝图事件观察者（SetEmitObserver 注入，须非阻塞）。
var emitObserver func(topic string, payload any)

// ==== 包根目录 ====

// packageRoot 计算 LTP9 包根目录（可执行目录/local_data/package）。
// 扫描顶层 local_data/package/，按 metadata.json 的 LTP9 标签筛选插件。
func packageRoot() string {
	execPath, err := os.Executable()
	if err != nil {
		return filepath.Join("local_data", "package")
	}
	execDir := filepath.Dir(execPath)
	return filepath.Join(execDir, config.LocalDir, "package")
}
