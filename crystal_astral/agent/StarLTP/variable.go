package StarLTP

// ==== 常量与变量集中区 ====

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/GeneralConfig"
)

// ServiceName 模块标识。
const ServiceName = "StarLTP"

// EngineBuild 引擎构建标记：每次改动 callFn/导出回传逻辑时递增，供运行实例自证版本。
const EngineBuild = "2026-09-08"

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

// agentInvoker 前端智能体调用实现（engine.agent 真实通道，由主机注入）。
var agentInvoker func(appID, instruction string) (string, error)

// sendMu 保护 sendInvoker 的并发读写。
var sendMu sync.RWMutex

// sendInvoker 发送通道（engine.send 真实实现，由主机注入）：kind=image/text/hybrid。
var sendInvoker func(kind, target string, payload any) error

// wsMu 保护 wsServicer 的并发读写。
var wsMu sync.RWMutex

// wsServicer WebSocket 服务端传输（engine.ws 真实实现，由主机注入）。
var wsServicer *WsBridge

// ==== 模型/算法配置（从 lunar_config.json 的 agent 字段读取，不硬编码） ====

// packageRoot 计算 LTP9 包根目录（可执行目录/local_data/package）。
// 扫描顶层 local_data/package/，按 metadata.json 的 LTP9 标签筛选插件。
func packageRoot() string {
	execPath, err := os.Executable()
	if err != nil {
		return filepath.Join("local_data", "package")
	}
	execDir := filepath.Dir(execPath)
	return filepath.Join(execDir, *GeneralConfig.LocalDir, "package")
}