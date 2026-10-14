//go:build windows

package TuiSystem

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// initSampler 初始化采样状态（Windows 进程伪句柄）
func initSampler() {
	winProcHandle = uintptr(windows.CurrentProcess())
}

// samplerLoop 状态采样主循环：每秒刷新 CPU / 内存快照
func samplerLoop() {
	for range time.Tick(statsInterval) {
		sampleOnce()
	}
}

// sampleOnce 执行一次采样并写入最新快照
func sampleOnce() {
	now := time.Now()

	// ==== 进程 CPU 时间（用户态 + 内核态，FILETIME 100ns 单位） ====
	var creation, exit, kernel, user windows.Filetime
	cpuPercent := 0.0
	if err := windows.GetProcessTimes(windows.Handle(winProcHandle), &creation, &exit, &kernel, &user); err == nil {
		procTime := kernel.Nanoseconds() + user.Nanoseconds()
		if !lastSampleAt.IsZero() {
			wallNanos := now.Sub(lastSampleAt).Nanoseconds()
			if wallNanos > 0 {
				prev := atomic.LoadInt64(&lastProcTimes)
				cpuPercent = float64(procTime-prev) / float64(wallNanos) / float64(runtime.NumCPU()) * 100
				if cpuPercent < 0 {
					cpuPercent = 0
				}
				if cpuPercent > 100 {
					cpuPercent = 100
				}
			}
		}
		atomic.StoreInt64(&lastProcTimes, procTime)
	}

	// ==== 进程工作集（psapi GetProcessMemoryInfo） ====
	workingSetMB := 0.0
	if mem := queryWorkingSet(); mem > 0 {
		workingSetMB = float64(mem) / (1024 * 1024)
	}

	// ==== Go 堆与 goroutine ====
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	// ==== 系统可用物理内存（kernel32 GlobalMemoryStatusEx） ====
	sysAvailMB := 0.0
	if mem := queryAvailPhys(); mem > 0 {
		sysAvailMB = float64(mem) / (1024 * 1024)
	}

	snap := Snapshot{
		Uptime:       time.Since(startTime),
		CPUPercent:   cpuPercent,
		WorkingSetMB: workingSetMB,
		HeapMB:       float64(ms.HeapAlloc) / (1024 * 1024),
		SysAvailMB:   sysAvailMB,
		Goroutines:   runtime.NumGoroutine(),
		Stats:        GetModelCallStats(),
	}

	statsMu.Lock()
	latestStats = snap
	statsMu.Unlock()
}

// PROCESS_MEMORY_COUNTERS psapi 进程内存计数器（uintptr 尺寸字段对齐）
type PROCESS_MEMORY_COUNTERS struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// MEMORYSTATUSEX kernel32 内存状态结构
type MEMORYSTATUSEX struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvilPhys             uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// psapi / kernel32 延迟加载（避免直接链接配置依赖）
var (
	psapi                = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemory = psapi.NewProc("GetProcessMemoryInfo")

	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemory = kernel32.NewProc("GlobalMemoryStatusEx")
)

// queryWorkingSet 返回当前进程工作集大小（字节；失败返回 0）
func queryWorkingSet() uintptr {
	if err := procGetProcessMemory.Find(); err != nil {
		return 0
	}
	var counters PROCESS_MEMORY_COUNTERS
	counters.CB = uint32(unsafe.Sizeof(counters))
	ret, _, _ := procGetProcessMemory.Call(
		winProcHandle,
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.CB),
	)
	if ret == 0 {
		return 0
	}
	return counters.WorkingSetSize
}

// queryAvailPhys 返回系统可用物理内存（字节；失败返回 0）
func queryAvailPhys() uint64 {
	if err := procGlobalMemory.Find(); err != nil {
		return 0
	}
	var status MEMORYSTATUSEX
	status.Length = uint32(unsafe.Sizeof(status))
	ret, _, _ := procGlobalMemory.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return 0
	}
	return status.AvilPhys
}
