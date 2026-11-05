//go:build windows

package desktop

// ==== 窗口定位与控制 ====
// 负责读取窗口标题/类名/进程信息、枚举/激活/关闭窗口。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
)

// windowText 读取窗口的标题文本。
func windowText(hwnd win.HWND) string {
	n, _, _ := procGetWindowTextLenW.Call(uintptr(hwnd))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return syscall.UTF16ToString(buf)
}

// windowClass 读取窗口的类名。
func windowClass(hwnd win.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// processName 读取窗口所属进程的可执行文件名。
func processName(hwnd win.HWND) string {
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	if pid == 0 {
		return ""
	}
	hProc, _, _ := procOpenProcess.Call(uintptr(0x1000), 0, uintptr(pid))
	if hProc == 0 {
		return ""
	}
	defer procCloseHandle.Call(hProc)
	buf := make([]uint16, 512)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessImageName.Call(hProc, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}
	return filepath.Base(syscall.UTF16ToString(buf[:size]))
}

// hwndMatches 判断窗口标题或进程名是否包含指定关键字（忽略大小写）。
func hwndMatches(hwnd win.HWND, key string) bool {
	t := strings.ToLower(windowText(hwnd))
	if t != "" && strings.Contains(t, key) {
		return true
	}
	p := strings.ToLower(processName(hwnd))
	if p != "" && strings.Contains(p, key) {
		return true
	}
	return false
}

// ListWindows 枚举所有可见且有标题的顶层窗口，按标题排序返回。
func ListWindows() []Window {
	var wins []Window
	cb := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		if !win.IsWindowVisible(hwnd) {
			return 1
		}
		title := strings.TrimSpace(windowText(hwnd))
		if title == "" {
			return 1
		}
		var pid uint32
		win.GetWindowThreadProcessId(hwnd, &pid)
		wins = append(wins, Window{Title: title, Class: windowClass(hwnd), PID: pid, Process: processName(hwnd)})
		return 1
	})
	procEnumWindows.Call(cb, 0)
	// 预先计算小写标题，避免排序比较器重复分配
	for i := range wins {
		wins[i].lowerTitle = strings.ToLower(wins[i].Title)
	}
	sort.SliceStable(wins, func(i, j int) bool { return wins[i].lowerTitle < wins[j].lowerTitle })
	return wins
}

// ForegroundTitle 返回当前前台窗口的标题文本。
func ForegroundTitle() string {
	return strings.TrimSpace(windowText(win.GetForegroundWindow()))
}

// ForegroundProcess 返回当前前台窗口所属进程的可执行文件名。
func ForegroundProcess() string {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return ""
	}
	return processName(hwnd)
}

// ForegroundMatches 当前前台窗口的标题或进程名是否包含指定关键字（含别名展开）。
func ForegroundMatches(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return false
	}
	for _, alias := range programAliases(key) {
		if hwndMatches(hwnd, alias) {
			return true
		}
	}
	return false
}

// ForegroundIsSelf 当前前台窗口是否属于本进程（控制台 / 月华 / 琉璃等自有窗口）。
// 桌面操纵不能把键鼠打进自己的控制台，否则输入会被 TUI 吞掉甚至误触菜单。
func ForegroundIsSelf() bool {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return false
	}
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	return pid != 0 && pid == uint32(os.Getpid())
}

// forceForeground 通过强制手段将指定窗口置顶并设为前台、激活。
func forceForeground(hwnd win.HWND) {
	procKeybdEvent.Call(uintptr(win.VK_MENU), 0, 0, 0)
	procKeybdEvent.Call(uintptr(win.VK_MENU), 0, mswKeyup, 0)
	time.Sleep(30 * time.Millisecond)
	curThread := win.GetCurrentThreadId()
	targetThread := win.GetWindowThreadProcessId(hwnd, nil)
	win.AttachThreadInput(int32(curThread), int32(targetThread), true)
	win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	win.ShowWindow(hwnd, win.SW_RESTORE)
	win.SetActiveWindow(hwnd)
	win.SetForegroundWindow(hwnd)
	win.BringWindowToTop(hwnd)
	win.SetFocus(hwnd)
	win.AttachThreadInput(int32(curThread), int32(targetThread), false)
	win.SetWindowPos(hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	time.Sleep(120 * time.Millisecond)
}

// ActivateWindow 激活标题/进程名含指定关键字的窗口；若已在前台直接返回成功。
func ActivateWindow(title string) error {
	target := strings.ToLower(strings.TrimSpace(title))
	if target == "" {
		return fmt.Errorf("窗口标题关键字为空")
	}
	if fg := win.GetForegroundWindow(); fg != 0 && hwndMatches(fg, target) {
		return nil
	}
	var found win.HWND
	cb := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		if !win.IsWindowVisible(hwnd) {
			return 1
		}
		if hwndMatches(hwnd, target) {
			found = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if found == 0 {
		return fmt.Errorf("未找到标题含「%s」的窗口", title)
	}
	if found == win.GetForegroundWindow() {
		return nil
	}
	forceForeground(found)
	time.Sleep(120 * time.Millisecond)
	fg2 := win.GetForegroundWindow()
	if fg2 == 0 || (fg2 != found && !hwndMatches(fg2, target)) {
		return fmt.Errorf("未能将「%s」置于前台（焦点可能被系统前台锁拦截），请先手动点击该窗口后重试", title)
	}
	return nil
}

// ActivateLaunchByName 启动某程序后，把匹配关键字的可见窗口立即置前台（通用「启动即置前台」）。
// 避免应用进程已拉起、但窗口迟迟不跳到前台、用户只觉得“停住”的体感问题。
// 跳过终端/控制台窗口，防止把新窗口和前台的判定落到终端上；中文别名会展开为实际进程名。
func ActivateLaunchByName(key string) string {
	base := strings.ToLower(strings.TrimSpace(key))
	if base == "" {
		return ""
	}
	keys := programAliases(base)
	time.Sleep(900 * time.Millisecond) // 等窗口出现
	for _, k := range keys {
		for _, w := range ListWindows() {
			if terminalProcs[strings.ToLower(w.Process)] {
				continue
			}
			if strings.Contains(strings.ToLower(w.Title), k) || strings.Contains(strings.ToLower(w.Process), k) {
				if err := ActivateWindow(w.Title); err == nil {
					return w.Title
				}
			}
		}
	}
	return ""
}

// CloseWindow 向标题含关键字的全部可见顶层窗口发送 WM_CLOSE 关闭请求。
func CloseWindow(title string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(title))
	if target == "" {
		return "", fmt.Errorf("窗口标题关键字为空")
	}
	var closed []string
	cb := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		if !win.IsWindowVisible(hwnd) {
			return 1
		}
		if hwndMatches(hwnd, target) {
			win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
			closed = append(closed, windowText(hwnd))
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if len(closed) == 0 {
		return "", fmt.Errorf("未找到标题含「%s」的窗口", title)
	}
	return fmt.Sprintf("已向 %d 个窗口发送关闭请求：%s", len(closed), strings.Join(closed, "、")), nil
}
