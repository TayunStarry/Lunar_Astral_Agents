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

// EngineBuild 引擎构建标记：每次改动沙箱绑定/回调执行逻辑时递增，供运行实例自证版本。
const EngineBuild = "2026-09-16"

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

// StickerCollection LTP9 图片记忆集合名（image 型集合，复用项目记忆库的 stickers 约定）。
const StickerCollection = "stickers"

// ==== 包根目录 ====

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
