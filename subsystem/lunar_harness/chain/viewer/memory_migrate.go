package viewer

// memory_migrate.go 观影记忆库的启动准备。
//
// 存储与历史迁移已下沉到 engine/viewerstore；这里只保留启动时序与日志：
// 先初始化库，再执行一次性迁移。迁移失败只记日志：观影继续可用，
// 记忆退化为本次运行内的内存态（下次启动会重试迁移）。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// prepareViewerMemory 启动时初始化观影记忆库并执行一次性迁移。
func prepareViewerMemory() {
	if err := viewerstore.Init(); err != nil {
		logger.Warn("BilibiliViewer", "初始化观影记忆库失败: %v", err)
		return
	}
	report, err := viewerstore.MigrateLegacyMemories(viewerMemorySource)
	if report.Source > 0 {
		logger.Info("BilibiliViewer", "观影记忆迁移对账：源 %d 条，成功入库 %d 条，丢弃 %d 条，失败 %d 条，备份 %s",
			report.Source, report.Imported, report.Discarded, report.Failed, report.BackupPath)
	}
	if err != nil {
		logger.Warn("BilibiliViewer", "迁移历史观影记忆未完成: %v", err)
	}
}
