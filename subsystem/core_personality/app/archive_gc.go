package app

// archive_gc.go 统一资源仓库的周期回收守护
//
// 设计依据：docs/adr/0001「保留策略与降级语义」。
// 回收只删 blob、保留资源目录行：句柄 @D.003 / @I.007 依然可解析，
// 取用时降级为描述（图片）或大纲（文档）。

import (
	"time"

	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// startArchiveCollector 启动资源仓库回收守护（间隔为 0 时关闭周期回收）
func startArchiveCollector() {
	intervalMin := *GeneralConfig.ArchiveGCIntervalMin
	if intervalMin <= 0 {
		LoggerGeneral.Info("CorePersonality", "资源仓库周期回收已关闭（archive-gc-interval-min=0）")
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				// 回收过程中任何 panic 不得终止守护
				defer func() {
					if r := recover(); r != nil {
						LoggerGeneral.Error("CorePersonality", "资源仓库回收异常: %v", r)
					}
				}()
				if _, err := file.ArchiveCollect(false); err != nil {
					LoggerGeneral.Warn("CorePersonality", "资源仓库回收失败: %v", err)
				}
			}()
		}
	}()
	LoggerGeneral.Info("CorePersonality", "资源仓库回收守护已启动（每 %d 分钟，配额 %d MB）",
		intervalMin, *GeneralConfig.ArchiveQuotaMB)
}
