package resource

// gc_runtime.go 运行期 GC：不再依赖进程重启。
//
// 两个节奏分开：
//   - 配额回收（保留等级 + 最久未用）按 config.ArchiveGCIntervalMin 执行（默认 30 分钟）；
//   - 积分结算（未用 -1 / 取用 +1，-20 回收）按 GCCycleHours 执行（默认 12 小时）。
//
// 启动 GC 负责孤儿扫描与全库建档；这里补上「长时间不重启也要回收」的缺口。

import (
	"fmt"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

var (
	// gcDaemonMu 保护运行期 GC 协程的启停
	gcDaemonMu sync.Mutex
	// gcDaemonStop 非空表示协程正在运行；关闭该通道即退出
	gcDaemonStop chan struct{}
)

// startRuntimeGCDaemon 启动周期 GC（重复调用幂等）。
func startRuntimeGCDaemon() {
	gcDaemonMu.Lock()
	defer gcDaemonMu.Unlock()
	if gcDaemonStop != nil {
		return
	}
	stop := make(chan struct{})
	gcDaemonStop = stop
	go loopRuntimeGC(stop)
	logger.Info("Persistence", "运行期 GC 已启动：配额回收每 %d 分钟，积分结算每 %d 小时",
		config.ArchiveGCIntervalMin, GCCycleHours)
}

// stopRuntimeGCDaemon 停止周期 GC（进程关闭或句柄库关闭时调用）。
func stopRuntimeGCDaemon() {
	gcDaemonMu.Lock()
	defer gcDaemonMu.Unlock()
	if gcDaemonStop == nil {
		return
	}
	close(gcDaemonStop)
	gcDaemonStop = nil
}

// loopRuntimeGC 按两个独立节奏驱动回收。
func loopRuntimeGC(stop chan struct{}) {
	scoreTicker := time.NewTicker(gcRuntimeInterval)
	defer scoreTicker.Stop()

	var quotaC <-chan time.Time
	if config.ArchiveGCIntervalMin > 0 {
		quotaTicker := time.NewTicker(time.Duration(config.ArchiveGCIntervalMin) * time.Minute)
		defer quotaTicker.Stop()
		quotaC = quotaTicker.C
	}

	for {
		select {
		case <-stop:
			return
		case <-quotaC:
			if err := RunQuotaGC(); err != nil {
				logger.Warn("Persistence", "配额回收失败: %v", err)
			}
		case <-scoreTicker.C:
			if err := RunRuntimeGC(); err != nil {
				logger.Warn("Persistence", "积分结算失败: %v", err)
			}
		}
	}
}

// RunRuntimeGC 执行一次完整结算：失效档案 → 积分 → 新档案登记 → 配额回收。
func RunRuntimeGC() error {
	if err := EnsureReady(); err != nil {
		return err
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	if archiveDB == nil || archiveDB.db == nil {
		return fmt.Errorf("句柄库未初始化")
	}
	pruned := archiveDB.pruneGCMediaScoresLocked()
	scored := archiveDB.applyGCScoringLocked()
	tracked := archiveDB.seedGCMediaScoresLocked()
	quota := archiveDB.runQuotaGCLocked()
	logger.Info("Persistence", "运行期 GC 完成：可清理媒体=%d 失效档案=%d 积分回收=%d 配额回收=%d",
		tracked, pruned, scored, quota)
	return nil
}

// RunQuotaGC 只执行配额回收（不动积分，避免 30 分钟一次的结算过快）。
func RunQuotaGC() error {
	if err := EnsureReady(); err != nil {
		return err
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	if archiveDB == nil || archiveDB.db == nil {
		return fmt.Errorf("句柄库未初始化")
	}
	archiveDB.runQuotaGCLocked()
	return nil
}
