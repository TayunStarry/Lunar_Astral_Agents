//go:build !windows

package console

import (
	"runtime"
	"time"
)

// initSampler 非 Windows 平台无额外初始化
func initSampler() {}

// samplerLoop 非 Windows 平台仅刷新运行时间与 Go 运行时指标
func samplerLoop() {
	for range time.Tick(statsInterval) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		snap := Snapshot{
			Uptime:     time.Since(startTime),
			HeapMB:     float64(ms.HeapAlloc) / (1024 * 1024),
			Goroutines: runtime.NumGoroutine(),
			Stats:      GetModelCallStats(),
		}

		statsMu.Lock()
		latestStats = snap
		statsMu.Unlock()
	}
}
