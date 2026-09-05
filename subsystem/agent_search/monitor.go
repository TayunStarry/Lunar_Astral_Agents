package AgentSearch

import (
	"LunarSubsystem/LoggerGeneral"
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// =============================================================================
// 浏览器资源监控 — 基于 gopsutil 的进程健康检查
// =============================================================================

func init() {
	// 注册浏览器健康检查钩子到 agent.go
	checkBrowserHealth = monitorBrowserHealth
}

// =============================================================================
// 浏览器健康检查
// =============================================================================

// monitorBrowserHealth 检查浏览器进程健康状态
// 判定规则（综合判断）：
//   - 浏览器进程不存在 → 不健康
//   - 内存占用 > BrowserMaxMemMB → 不健康
//   - CPU 持续 > BrowserMaxCPUPercent 超过 BrowserCPUHighDuration → 不健康
//   - 查询超时（由 agent.go 层处理，此处仅检查资源）
func monitorBrowserHealth() BrowserHealth {
	health := BrowserHealth{
		QueryCount: browserQueryCount,
	}

	// 查找 Chrome 浏览器进程
	chromeProcs := findChromeProcesses()
	if len(chromeProcs) == 0 {
		health.IsRunning = false
		health.Healthy = false
		LoggerGeneral.Info(ModuleName, "浏览器健康检查: 未找到 Chrome 进程\n")
		return health
	}

	health.IsRunning = true

	// 聚合内存占用
	var totalMemMB uint64
	for _, p := range chromeProcs {
		memInfo, err := p.MemoryInfo()
		if err != nil {
			continue
		}
		totalMemMB += memInfo.RSS / (1024 * 1024) // 字节 → MB
	}
	health.MemMB = totalMemMB

	// 聚合 CPU 占用
	var totalCPU float64
	var cpuCount int
	for _, p := range chromeProcs {
		cpuPercent, err := p.CPUPercent()
		if err != nil {
			continue
		}
		totalCPU += cpuPercent
		cpuCount++
	}
	if cpuCount > 0 {
		health.CPUPercent = totalCPU // 多进程 CPU 累加（单核百分比）
	}

	// 追踪 CPU 持续高占用
	trackCPUHigh(health.CPUPercent)

	// 综合判定
	health.Healthy = true

	if totalMemMB > BrowserMaxMemMB {
		health.Healthy = false
		LoggerGeneral.Info(ModuleName, "浏览器内存超标: %dMB > %dMB\n", totalMemMB, BrowserMaxMemMB)
	}

	if isCPUSustainedHigh() {
		health.Healthy = false
		LoggerGeneral.Info(ModuleName, "浏览器 CPU 持续高占用: %.1f%% > %.0f%% 已持续 >= %v\n", health.CPUPercent, BrowserMaxCPUPercent, BrowserCPUHighDuration)
	}

	return health
}

// findChromeProcesses 查找所有 Chrome 浏览器进程
// 在 Windows 上匹配 chrome.exe，在 Linux/macOS 上匹配 chrome
func findChromeProcesses() []*process.Process {
	procs, err := process.Processes()
	if err != nil {
		LoggerGeneral.Info(ModuleName, "获取进程列表失败: %v\n", err)
		return nil
	}

	var chromeProcs []*process.Process
	chromeNames := chromeProcessNames()

	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}

		for _, chromeName := range chromeNames {
			if name == chromeName {
				chromeProcs = append(chromeProcs, p)
				break
			}
		}
	}

	return chromeProcs
}

// chromeProcessNames 返回当前平台的浏览器进程名（Chrome + Edge 均支持）
func chromeProcessNames() []string {
	// chromedp 在 Windows 上支持 Chrome 和 Edge（二者均为 Chromium 内核）
	// Headless 模式下进程名与普通模式一致
	return []string{
		"chrome.exe",
		"chromium.exe",
		"msedge.exe",       // Microsoft Edge
		"chrome",           // Linux/macOS
		"chromium",         // Linux/macOS
		"chromium-browser", // Linux 变体
		"msedge",           // Linux Edge
	}
}

// =============================================================================
// CPU 持续高占用追踪
// =============================================================================

// trackCPUHigh 追踪 CPU 持续高占用状态
func trackCPUHigh(currentCPU float64) {
	cpuHighMu.Lock()
	defer cpuHighMu.Unlock()

	now := time.Now()

	// 记录本次检查
	cpuCheckHistory = append(cpuCheckHistory, cpuReading{
		percent:   currentCPU,
		timestamp: now,
	})
	if len(cpuCheckHistory) > maxCPUHistory {
		cpuCheckHistory = cpuCheckHistory[len(cpuCheckHistory)-maxCPUHistory:]
	}

	isHigh := currentCPU > BrowserMaxCPUPercent

	if isHigh {
		if !cpuWasHigh {
			// CPU 首次超过阈值，记录时间
			cpuHighSince = now
		}
		// 如果一直高，cpuHighSince 保持不变
	} else {
		// CPU 回落到阈值以下，重置
		cpuHighSince = time.Time{} // 零值
	}

	cpuWasHigh = isHigh
}

// isCPUSustainedHigh 判断 CPU 是否持续高占用超过阈值时间
func isCPUSustainedHigh() bool {
	cpuHighMu.Lock()
	defer cpuHighMu.Unlock()

	if cpuHighSince.IsZero() {
		return false
	}

	return time.Since(cpuHighSince) >= BrowserCPUHighDuration
}

// =============================================================================
// 工具函数
// =============================================================================

// GetBrowserPID 获取当前 Go 进程的 PID（用于调试）
func GetBrowserPID() int {
	return os.Getpid()
}

// ResetCPUTracking 重置 CPU 追踪状态（浏览器重启后调用）
func ResetCPUTracking() {
	cpuHighMu.Lock()
	defer cpuHighMu.Unlock()

	cpuHighSince = time.Time{}
	cpuWasHigh = false
	cpuCheckHistory = nil
}
