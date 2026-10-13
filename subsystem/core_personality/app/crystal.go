package app

import (
	"context"
	"net/http"
	"time"

	"CorePersonality/personality/crystal"
	"CorePersonality/protocol/ltpx"
	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// assembleCrystal 装配琉璃人格格：知识库/记忆库、LTP9 插件引擎与前端总线消费。
func assembleCrystal() error {
	// ==== 知识库（SQLite） ====
	if err := file.InitKnowledgeDB(*GeneralConfig.KnowledgeDBPath); err != nil {
		LoggerGeneral.Warn("CorePersonality", "知识库初始化失败: %v (不影响服务启动)", err)
	}

	// ==== 记忆库存储目录（仅准备本地存储结构，不产生网络请求） ====
	file.InitMemoryDB(*GeneralConfig.MemoryDBDir)

	// ==== 记忆库实例与默认集合（与月华 JS agent 行为对齐） ====
	initMemoryDatabase()

	// ==== LTP9-Flash 引擎：注入宿主通道（前端智能体 / 单向通报 / 调试回执推送） ====
	if err := crystal.BridgeLTP9(); err != nil {
		LoggerGeneral.Warn("CorePersonality", "LTP9 引擎初始化失败: %v (不影响服务启动)", err)
	}

	// ==== LTPX 协调：注入进程内实现，月华侧不再走 HTTP 通道 ====
	ltpx.SetBroker(crystal.NewBroker())
	LoggerGeneral.Info("CorePersonality", "LTPX 协调层已进程内化")

	return nil
}

// initMemoryDatabase 自动初始化记忆库实例与默认集合（模型配置从 lunar_config.json 读取）。
// 失败仅打印警告，不阻断服务启动，用户仍可通过记忆库面板手动初始化。
func initMemoryDatabase() {
	if err := file.MemoryInitInstance(); err != nil {
		LoggerGeneral.Warn("CorePersonality", "记忆库实例初始化失败: %v (可手动通过记忆库面板初始化)", err)
		return
	}
	LoggerGeneral.Info("CorePersonality", "记忆库实例初始化完成 (模型配置从 lunar_config.json 读取)")

	// 探针文本嵌入定维度，含网络请求，加超时保护
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := file.CollectionInit(ctx, crystal.DefaultMemoryCollection, crystal.DefaultMemoryModelName); err != nil {
		LoggerGeneral.Warn("CorePersonality", "集合 [%s] 创建失败: %v (可手动通过记忆库面板初始化)", crystal.DefaultMemoryCollection, err)
		return
	}
	LoggerGeneral.Info("CorePersonality", "集合 [%s] 创建成功, 模型: %s", crystal.DefaultMemoryCollection, crystal.DefaultMemoryModelName)
}

// crystalStaticRoot 琉璃桌面的静态资源根（内嵌 web 目录子树）
func crystalStaticRoot() http.FileSystem {
	return crystal.Gethierarchy()
}
