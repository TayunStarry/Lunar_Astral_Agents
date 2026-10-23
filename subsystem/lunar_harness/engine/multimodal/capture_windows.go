//go:build windows

package multimodal

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"LunarSubsystem/LunarHarness/engine/capture"
	"github.com/lxn/win"
)

// Win32 过程地址（与截图相关的窗口定位/枚举能力）
// 说明：这些句柄依赖 Windows 平台，故与平台实现同文件；跨平台可用的类型与状态在 type.go / variable.go。
var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procIsWindow               = user32.NewProc("IsWindow")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procOpenProcess            = kernel32.NewProc("OpenProcess")
	procQueryFullProcessName   = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle            = kernel32.NewProc("CloseHandle")
)

// processQueryLimitedInformation 查询进程信息所需的最小权限
const processQueryLimitedInformation = 0x1000

var (
	// windowEnumMu 保护枚举回调写入的切片（EnumWindows 是同步回调，锁住调用窗口即可）
	windowEnumMu sync.Mutex
	// windowEnumResult 当前枚举批次的结果
	windowEnumResult []WindowInfo
)

// enumWindowsCallback 顶层窗口枚举回调（回调只注册一次，避免 syscall 回调槽泄漏）
var enumWindowsCallback = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
	if info, ok := readWindowInfo(hwnd); ok {
		windowEnumResult = append(windowEnumResult, info)
	}
	return 1
})

// getWindowTitle 获取窗口标题
func getWindowTitle(hwnd uintptr) string {
	buf := make([]uint16, 512)
	ret, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:int(ret)])
}

// isWindow 判断句柄是否仍是有效窗口
func isWindow(hwnd uintptr) bool {
	ret, _, _ := procIsWindow.Call(hwnd)
	return ret != 0
}

// windowProcessName 取窗口所属进程的可执行文件名（失败时返回空串）
func windowProcessName(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer procCloseHandle.Call(handle)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessName.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}
	return filepath.Base(syscall.UTF16ToString(buf[:size]))
}

// readWindowInfo 读取单个窗口的可截图信息；不可见/无标题/尺寸非法的辅助窗口返回 false
func readWindowInfo(hwnd uintptr) (WindowInfo, bool) {
	if !win.IsWindowVisible(win.HWND(hwnd)) {
		return WindowInfo{}, false
	}
	title := getWindowTitle(hwnd)
	if strings.TrimSpace(title) == "" {
		return WindowInfo{}, false
	}
	var rect win.RECT
	if !win.GetWindowRect(win.HWND(hwnd), &rect) {
		return WindowInfo{}, false
	}
	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)
	if width <= 0 || height <= 0 {
		return WindowInfo{}, false
	}
	var client win.RECT
	clientW, clientH := 0, 0
	if win.GetClientRect(win.HWND(hwnd), &client) {
		clientW = int(client.Right - client.Left)
		clientH = int(client.Bottom - client.Top)
	}
	className := make([]uint16, 256)
	class := ""
	if n, err := win.GetClassName(win.HWND(hwnd), &className[0], len(className)); err == nil && n > 0 {
		class = syscall.UTF16ToString(className[:n])
	}
	pid := uint32(0)
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return WindowInfo{
		Handle:     uint64(hwnd),
		Title:      title,
		Class:      class,
		ProcessID:  pid,
		Process:    windowProcessName(pid),
		X:          int(rect.Left),
		Y:          int(rect.Top),
		Width:      width,
		Height:     height,
		ClientW:    clientW,
		ClientH:    clientH,
		Minimized:  win.IsIconic(win.HWND(hwnd)),
		Foreground: win.HWND(hwnd) == win.GetForegroundWindow(),
	}, true
}

// listWindows 枚举全部可见顶层窗口
func listWindows() ([]WindowInfo, error) {
	windowEnumMu.Lock()
	defer windowEnumMu.Unlock()
	windowEnumResult = windowEnumResult[:0]
	ret, _, err := procEnumWindows.Call(enumWindowsCallback, 0)
	if ret == 0 {
		return nil, fmt.Errorf("枚举窗口失败: %v", err)
	}
	result := make([]WindowInfo, len(windowEnumResult))
	copy(result, windowEnumResult)
	return result, nil
}

// findWindowByTitle 按标题包含匹配（忽略大小写）查找窗口，优先焦点窗口与完全同名窗口
func findWindowByTitle(title string) (uintptr, string, error) {
	keyword := strings.ToLower(strings.TrimSpace(title))
	if keyword == "" {
		return 0, "", fmt.Errorf("窗口标题不能为空")
	}
	windows, err := listWindows()
	if err != nil {
		return 0, "", err
	}
	var (
		firstMatch uintptr
		firstTitle string
	)
	for _, item := range windows {
		lower := strings.ToLower(item.Title)
		if !strings.Contains(lower, keyword) {
			continue
		}
		if item.Handle == 0 {
			continue
		}
		if lower == keyword || (item.Foreground && firstMatch == 0) {
			return uintptr(item.Handle), item.Title, nil
		}
		if firstMatch == 0 {
			firstMatch = uintptr(item.Handle)
			firstTitle = item.Title
		}
	}
	if firstMatch == 0 {
		return 0, "", fmt.Errorf("未找到标题包含「%s」的窗口（可用 ListWindows 查看可选目标）", title)
	}
	return firstMatch, firstTitle, nil
}

// resolveTargetWindow 解析截图目标窗口：句柄 > 标题 > 当前焦点窗口
func resolveTargetWindow(title string, handle uint64) (uintptr, string, error) {
	if handle != 0 {
		hwnd := uintptr(handle)
		if !isWindow(hwnd) {
			return 0, "", fmt.Errorf("窗口句柄 %d 已失效", handle)
		}
		if win.IsIconic(win.HWND(hwnd)) {
			return 0, "", fmt.Errorf("目标窗口已最小化，无法截图（请先恢复窗口）")
		}
		return hwnd, getWindowTitle(hwnd), nil
	}
	if strings.TrimSpace(title) != "" {
		hwnd, matchedTitle, err := findWindowByTitle(title)
		if err != nil {
			return 0, "", err
		}
		if win.IsIconic(win.HWND(hwnd)) {
			return 0, "", fmt.Errorf("目标窗口「%s」已最小化，无法截图（请先恢复窗口）", matchedTitle)
		}
		return hwnd, matchedTitle, nil
	}
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return 0, "", fmt.Errorf("未检测到焦点窗口")
	}
	if win.IsIconic(hwnd) {
		return 0, "", fmt.Errorf("焦点窗口已最小化，无法截图")
	}
	return uintptr(hwnd), getWindowTitle(uintptr(hwnd)), nil
}

// captureTargetWindowArea 捕获目标窗口（整窗或客户区），并支持窗口内相对子区域
func captureTargetWindowArea(hwnd uintptr, clientArea bool, offsetX, offsetY, width, height int) (*image.RGBA, error) {
	mode := capture.ModeWindow
	if clientArea {
		mode = capture.ModeClient
	}
	if width > 0 && height > 0 {
		return capture.CaptureRegion(hwnd, mode, offsetX, offsetY, width, height)
	}
	return capture.Capture(hwnd, mode)
}

// CaptureWindowRGBA 按窗口句柄截取原始 RGBA 图像（clientArea=true 时只取客户区）。
//
// 该入口与 Capture 共用 WindowCapture：PrintWindow 优先、纯色/失败时回退屏幕可见区域，
// 因此被遮挡（未最小化）的窗口也能拿到画面。供需要自行编码的调用方（如浏览器会话截图）复用，
// 使窗口截图能力统一收敛到本模块。
func CaptureWindowRGBA(hwnd uintptr, clientArea bool) (*image.RGBA, error) {
	if hwnd == 0 {
		return nil, fmt.Errorf("窗口句柄为空")
	}
	if !isWindow(hwnd) {
		return nil, fmt.Errorf("窗口句柄已失效")
	}
	if win.IsIconic(win.HWND(hwnd)) {
		return nil, fmt.Errorf("目标窗口已最小化，无法截图（请先恢复窗口）")
	}
	return captureTargetWindowArea(hwnd, clientArea, 0, 0, 0, 0)
}
