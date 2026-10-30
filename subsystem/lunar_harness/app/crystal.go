package app

import (
	"net/http"

	"LunarSubsystem/LunarHarness/clients"
	"LunarSubsystem/LunarHarness/crystal"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/protocol/ltpx"
)

// assembleCrystal 装配琉璃人格格：知识库/记忆库、LTPX 协调与前端总线消费。
func assembleCrystal() error {
	// ==== 统一句柄库（SQLite handles.db：URL 主表 / 六类句柄 / 段落 / 缓存） ====
	// 必须先于月华智能体运行时启动：入库层在对话链路里即被调用。
	if err := resource.Init(config.ArchiveDBPath); err != nil {
		logger.Warn("LunarHarness", "统一句柄库初始化失败: %v (不影响服务启动)", err)
	}

	// ==== 知识库（SQLite，仅用户/插件自建表） ====
	if err := knowledge.InitKnowledge(config.KnowledgeDBPath); err != nil {
		logger.Warn("LunarHarness", "知识库初始化失败: %v (不影响服务启动)", err)
	}

	// ==== 领域数据库（text / character） ====
	// 句柄相关数据全部落在 handles.db；这里只建剩余领域库并提前暴露打开失败。
	if err := knowledge.Init(); err != nil {
		logger.Warn("LunarHarness", "领域数据库初始化存在失败项: %v", err)
	}

	// ==== 记忆库存储目录（仅准备本地存储结构，不产生网络请求） ====
	memory.Init(config.MemoryDBDir)

	// ==== 记忆库实例与默认分组（与月华 JS agent 行为对齐） ====
	initMemoryDatabase()

	// ==== LTPX 协调：注入进程内实现，月华侧不再走 HTTP 通道 ====
	ltpx.SetBroker(crystal.NewBroker())
	logger.Info("LunarHarness", "LTPX 协调层已进程内化")

	return nil
}

// initMemoryDatabase 初始化记忆库实例：一次性加载全部集合，并补齐默认分组与集合名向量。
// 模型配置与月华共用 lunar_config.json 的 agent 分组；失败仅告警，不阻断服务启动。
func initMemoryDatabase() {
	if err := memory.EnsureReady(); err != nil {
		logger.Warn("LunarHarness", "记忆库初始化失败: %v（可稍后通过 /memory/init 重试）", err)
		return
	}
	logger.Info("LunarHarness", "记忆库初始化完成（全部集合已加载，默认分组与集合名向量已就绪）")
}

// crystalStaticRoot 琉璃桌面的静态资源根（clients/crystal 子树，含 file-runner 编辑运行等内置子页）
func crystalStaticRoot() http.FileSystem {
	return clients.CrystalFS()
}
