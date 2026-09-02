//go:build windows

package FaceLTP

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	imageproc "LunarSubsystem/ImageProcessor/module"
	"github.com/lxn/win"
)

// ==== Win32 API 惰性加载（lxn/win 未覆盖的部分） ====

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	procEnumWindows       = user32.NewProc("EnumWindows")
	procGetWindowTextW    = user32.NewProc("GetWindowTextW")
	procGetWindowTextLenW = user32.NewProc("GetWindowTextLengthW")
	procGetClassNameW     = user32.NewProc("GetClassNameW")
	procMouseEvent        = user32.NewProc("mouse_event")
	procKeybdEvent        = user32.NewProc("keybd_event")

	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
)

// ==== 鼠标事件标志 ====

const (
	moeEventfLeftdown   = 0x0002
	moeEventfLeftup     = 0x0004
	moeEventfRightdown  = 0x0008
	moeEventfRightup    = 0x0010
	moeEventfMiddledown = 0x0020
	moeEventfMiddleup   = 0x0040
	moeEventfWheel      = 0x0800
	wheelDelta          = 120
	keyeventfKeyup      = 0x0002
)

// ==== 窗口枚举与文案 ====

// getWindowText 读取窗口标题（UTF-16）
func getWindowText(hwnd win.HWND) string {
	n, _, _ := procGetWindowTextLenW.Call(uintptr(hwnd))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return syscall.UTF16ToString(buf)
}

// getClassName 读取窗口类名（UTF-16）
func getClassName(hwnd win.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// getProcessName 通过 PID → 进程可执行文件名（如 QQ.exe），用于按进程名定位窗口
func getProcessName(hwnd win.HWND) string {
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	if pid == 0 {
		return ""
	}
	// PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	hProc, _, _ := procOpenProcess.Call(uintptr(0x1000), 0, uintptr(pid))
	if hProc == 0 {
		return ""
	}
	defer procCloseHandle.Call(hProc)
	buf := make([]uint16, 512)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessImageNameW.Call(hProc, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}
	return filepath.Base(syscall.UTF16ToString(buf[:size]))
}

// hwndMatches 判断窗口是否匹配关键字（窗口标题或进程名，大小写不敏感，子串匹配）
func hwndMatches(hwnd win.HWND, key string) bool {
	t := strings.ToLower(getWindowText(hwnd))
	if t != "" && strings.Contains(t, key) {
		return true
	}
	p := strings.ToLower(getProcessName(hwnd))
	if p != "" && strings.Contains(p, key) {
		return true
	}
	return false
}

// isTerminalWindow 判断窗口是否为终端/控制台窗口（按窗口类名或进程名识别）。
// 这类窗口不应作为 Face-LTP 的键入目标，用于防止焦点未落到目标程序时把文本误注入终端。
func isTerminalWindow(hwnd win.HWND) bool {
	switch strings.ToLower(getClassName(hwnd)) {
	case "consolewindowclass", "cascadia_hosting_window_class", "pseudoconsolewindow", "mintty", "putty":
		return true
	}
	switch strings.ToLower(getProcessName(hwnd)) {
	case "powershell.exe", "pwsh.exe", "cmd.exe", "conhost.exe", "windowsterminal.exe", "wt.exe", "bash.exe", "wsl.exe":
		return true
	}
	return false
}

// listWindows 枚举顶层可见窗口，返回带标题/类名/PID 的窗口列表
func listWindows() []windowInfo {
	var wins []windowInfo
	cb := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		if !win.IsWindowVisible(hwnd) {
			return 1
		}
		title := strings.TrimSpace(getWindowText(hwnd))
		if title == "" {
			return 1
		}
		var pid uint32
		win.GetWindowThreadProcessId(hwnd, &pid)
		wins = append(wins, windowInfo{Title: title, Class: getClassName(hwnd), PID: pid, Process: getProcessName(hwnd)})
		return 1
	})
	procEnumWindows.Call(cb, 0)
	sort.SliceStable(wins, func(i, j int) bool {
		return strings.ToLower(wins[i].Title) < strings.ToLower(wins[j].Title)
	})
	return wins
}

// getForegroundTitle 返回当前前台窗口标题
func getForegroundTitle() string {
	return strings.TrimSpace(getWindowText(win.GetForegroundWindow()))
}

// forceForeground 用多种手段强制将窗口置前台（Alt 键技巧 + 临时置顶 + AttachThreadInput + SetForegroundWindow）
func forceForeground(hwnd win.HWND) {
	// Alt 键技巧：后台进程点一下 Alt，让系统释放前台锁
	procKeybdEvent.Call(uintptr(win.VK_MENU), 0, 0, 0)
	procKeybdEvent.Call(uintptr(win.VK_MENU), 0, keyeventfKeyup, 0)
	time.Sleep(30 * time.Millisecond)

	// AttachThreadInput：让本线程与目标窗口线程共享输入队列，保证 SetForegroundWindow 对后台进程生效
	curThread := win.GetCurrentThreadId()
	targetThread := win.GetWindowThreadProcessId(hwnd, nil)
	win.AttachThreadInput(int32(curThread), int32(targetThread), true)

	// 临时置顶目的窗口以绕过前台锁，完成后恢复非置顶
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

// activateWindow 将标题含指定关键字的窗口置于前台，并验证前台是否真正切换成功
func activateWindow(title string) error {
	target := strings.ToLower(strings.TrimSpace(title))
	if target == "" {
		return fmt.Errorf("窗口标题关键字为空")
	}

	// 优先：当前前台窗口若已匹配关键字且非终端，直接返回、不做任何焦点操作（防止把用户已聚焦的窗口切跑）
	if fg := win.GetForegroundWindow(); fg != 0 && !isTerminalWindow(fg) && hwndMatches(fg, target) {
		return nil
	}

	var found win.HWND
	cb := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		if !win.IsWindowVisible(hwnd) {
			return 1
		}
		if hwndMatches(hwnd, target) {
			found = hwnd
			return 0 // 找到即停止枚举
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if found == 0 {
		return fmt.Errorf("未找到标题含「%s」的窗口", title)
	}
	// 目标窗口已是前台时直接返回，绝不再抢焦点（避免把已就绪的焦点切跑）
	if found == win.GetForegroundWindow() {
		return nil
	}
	forceForeground(found)

	// 验证前台是否真正切到目标窗口：Windows 前台锁可能拒绝后台进程抢焦点，
	// 若焦点仍停在终端等其他窗口上必须返回错误，避免后续键入被注入到错误窗口。
	time.Sleep(120 * time.Millisecond)
	fg2 := win.GetForegroundWindow()
	if fg2 == 0 || isTerminalWindow(fg2) || (fg2 != found && !hwndMatches(fg2, target)) {
		return fmt.Errorf("未能将「%s」置于前台（焦点可能被系统前台锁拦截），请先手动点击该窗口后重试", title)
	}
	return nil
}

// closeWindow 关闭标题含指定关键字的顶层窗口（发送 WM_CLOSE 关闭消息）
func closeWindow(title string) (string, error) {
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
			t := getWindowText(hwnd)
			win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
			closed = append(closed, t)
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if len(closed) == 0 {
		return "", fmt.Errorf("未找到标题含「%s」的窗口", title)
	}
	return fmt.Sprintf("已向 %d 个窗口发送关闭请求：%s", len(closed), strings.Join(closed, "、")), nil
}

// ==== 程序搜索与启动 ====

// knownProtocols 已知的 UWP/协议应用启动映射（名称关键字 → 要启动的协议 URI）
var knownProtocols = map[string]string{
	"store": "ms-windows-store:",
	"商店":    "ms-windows-store:",
	"商城":    "ms-windows-store:",
}

// matchProtocol 根据程序名关键字匹配已知协议（未匹配返回空串）
func matchProtocol(key string) string {
	for k, proto := range knownProtocols {
		if strings.Contains(key, k) {
			return proto
		}
	}
	return ""
}

// launchProgram 按名称搜索并启动程序；若同名窗口已存在则直接激活
func launchProgram(name string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "", fmt.Errorf("程序名关键字为空")
	}

	// 1. 先尝试激活已存在的同名窗口（按窗口标题或进程名匹配）
	wins := listWindows()
	for _, w := range wins {
		if strings.Contains(strings.ToLower(w.Title), key) || strings.Contains(strings.ToLower(w.Process), key) {
			if err := activateWindow(w.Title); err == nil {
				return "已激活现有窗口「" + w.Title + "」", nil
			}
		}
	}

	// 2. 搜索开始菜单与常见程序目录
	candidates := searchPrograms(key)
	if len(candidates) == 0 {
		// 2.1 UWP/协议应用兜底（如 Microsoft Store 之类没有普通 exe/lnk 的应用）
		if proto := matchProtocol(key); proto != "" {
			if err := exec.Command("cmd", "/c", "start", "", proto).Start(); err != nil {
				return "", fmt.Errorf("启动协议 %s 失败: %v", proto, err)
			}
			return "已通过协议启动 " + proto, nil
		}
		return "", fmt.Errorf("未找到与「%s」匹配的程序", name)
	}

	// 取最匹配的候选启动
	picked := candidates[0]
	if err := launchPath(picked); err != nil {
		return "", fmt.Errorf("启动「%s」失败: %v", picked, err)
	}
	return "已启动程序 " + picked, nil
}

// searchPrograms 在开始菜单与常见目录中查找名称包含 key 的可执行文件/快捷方式
func searchPrograms(key string) []string {
	var found []string
	seen := map[string]string{} // 归一化名 → 路径

	addPath := func(p string) {
		norm := strings.ToLower(filepath.Base(p))
		if _, ok := seen[norm]; ok {
			return
		}
		seen[norm] = p
		found = append(found, p)
	}

	// 开始菜单目录
	startDirs := []string{}
	if pd := os.Getenv("ProgramData"); pd != "" {
		startDirs = append(startDirs, filepath.Join(pd, "Microsoft", "Windows", "Start Menu", "Programs"))
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		startDirs = append(startDirs, filepath.Join(ad, "Microsoft", "Windows", "Start Menu", "Programs"))
	}

	for _, dir := range startDirs {
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".lnk" && ext != ".exe" && ext != ".url" && ext != ".bat" && ext != ".cmd" {
				return nil
			}
			base := strings.ToLower(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
			if strings.Contains(base, key) {
				addPath(p)
			}
			return nil
		})
	}

	// 常见安装目录（顶层子目录 + 其中直接 exe，避免深层递归开销）
	commonDirs := []string{
		`C:\Program Files`, `C:\Program Files (x86)`,
	}
	for _, dir := range commonDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				if strings.Contains(strings.ToLower(e.Name()), key) {
					sub := filepath.Join(dir, e.Name())
					subEntries, _ := os.ReadDir(sub)
					for _, se := range subEntries {
						if !se.IsDir() && strings.EqualFold(filepath.Ext(se.Name()), ".exe") {
							addPath(filepath.Join(sub, se.Name()))
						}
					}
				}
			} else if strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
				if strings.Contains(strings.ToLower(e.Name()), key) {
					addPath(filepath.Join(dir, e.Name()))
				}
			}
		}
	}

	// PATH 兜底（where 命令，Windows 大小写不敏感）
	if len(found) == 0 {
		if out, err := exec.Command("where", key).Output(); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					addPath(line)
				}
			}
		}
	}

	sort.StringSlice(found).Sort()
	return found
}

// launchPath 按扩展名分派启动
func launchPath(p string) error {
	ext := strings.ToLower(filepath.Ext(p))
	if ext == ".exe" {
		cmd := exec.Command(p)
		cmd.Dir = filepath.Dir(p)
		return cmd.Start()
	}
	cmd := exec.Command("cmd", "/c", "start", "", p)
	return cmd.Start()
}

// openFolder 直接在文件资源管理器中打开指定文件夹（一步到位，不依赖键盘焦点状态）
func openFolder(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("文件夹路径为空")
	}
	cmd := exec.Command("explorer.exe", p)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("打开文件夹失败: %v", err)
	}
	return "已在文件资源管理器打开文件夹 " + p, nil
}

// ==== 输入模拟 ====

// mouseEvent 调用 user32.mouse_event
func mouseEvent(flags, dx, dy, data uintptr) {
	procMouseEvent.Call(flags, dx, dy, data, 0)
}

// keybdEvent 调用 user32.keybd_event
func keybdEvent(vk uint16) {
	procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
}

// mouseClick 在屏幕绝对坐标 (x, y) 模拟点击
func mouseClick(x, y int, button string, double bool) error {
	if !win.SetCursorPos(int32(x), int32(y)) {
		return fmt.Errorf("移动光标到 (%d,%d) 失败", x, y)
	}
	time.Sleep(60 * time.Millisecond)

	down := uintptr(moeEventfLeftdown)
	up := uintptr(moeEventfLeftup)
	if strings.EqualFold(button, "right") {
		down = moeEventfRightdown
		up = moeEventfRightup
	}

	times := 1
	if double {
		times = 2
	}
	for i := 0; i < times; i++ {
		mouseEvent(down, 0, 0, 0)
		mouseEvent(up, 0, 0, 0)
		if times > 1 {
			time.Sleep(60 * time.Millisecond)
		}
	}
	return nil
}

// mouseButton 在屏幕绝对坐标按下并释放指定鼠标按键，支持按住时长（holdMs 毫秒）。
// 用于通用鼠标按键原语（mouse 工具）：右键/中键或按住语义（short=1s / long=10s）。
func mouseButton(x, y int, button string, holdMs int) error {
	if err := mouseMove(x, y); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	down, up := uintptr(moeEventfLeftdown), uintptr(moeEventfLeftup)
	switch strings.ToLower(button) {
	case "right":
		down, up = moeEventfRightdown, moeEventfRightup
	case "middle":
		down, up = moeEventfMiddledown, moeEventfMiddleup
	}
	mouseEvent(down, 0, 0, 0)
	if holdMs > 0 {
		time.Sleep(time.Duration(holdMs) * time.Millisecond)
	} else {
		time.Sleep(60 * time.Millisecond)
	}
	mouseEvent(up, 0, 0, 0)
	return nil
}

// mouseMove 在屏幕绝对坐标 (x, y) 移动鼠标光标
func mouseMove(x, y int) error {
	if !win.SetCursorPos(int32(x), int32(y)) {
		return fmt.Errorf("移动光标到 (%d,%d) 失败", x, y)
	}
	return nil
}

// mouseDrag 在屏幕绝对坐标间模拟按住左键拖拽（按下→分段移动→松开），用于画图/选择等拖拽操作。
// steps 为分段移动次数，越大越平滑。
func mouseDrag(x1, y1, x2, y2 int, steps int) error {
	if steps < 2 {
		steps = 2
	}
	if steps > 64 {
		steps = 64
	}
	if err := mouseMove(x1, y1); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	mouseEvent(moeEventfLeftdown, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)
	for i := 1; i <= steps; i++ {
		cx := x1 + (x2-x1)*i/steps
		cy := y1 + (y2-y1)*i/steps
		if err := mouseMove(cx, cy); err != nil {
			mouseEvent(moeEventfLeftup, 0, 0, 0)
			return err
		}
		time.Sleep(12 * time.Millisecond)
	}
	mouseEvent(moeEventfLeftup, 0, 0, 0)
	return nil
}

// windowCoordsDrag 将前台窗口内相对坐标的拖拽（起止两点）换算为屏幕绝对坐标后执行拖拽。
func windowCoordsDrag(wx1, wy1, wx2, wy2 int, steps int) error {
	sx1, sy1, err := windowCoordsToScreen(wx1, wy1)
	if err != nil {
		return err
	}
	sx2, sy2, err := windowCoordsToScreen(wx2, wy2)
	if err != nil {
		return err
	}
	return mouseDrag(sx1, sy1, sx2, sy2, steps)
}

// typeText 在当前焦点窗口按 Unicode 逐个键入文本（支持中文）
func typeText(text string) error {
	// 键入前校验：当前前台若是终端/控制台窗口则拒绝键入，防止焦点未落到目标程序时把内容误注入终端
	if fg := win.GetForegroundWindow(); fg != 0 && isTerminalWindow(fg) {
		return fmt.Errorf("当前前台是终端/控制台窗口，拒绝键入；请先 activate_window 激活目标程序窗口后再试")
	}
	for _, r := range text {
		var in win.KEYBD_INPUT
		in.Type = win.INPUT_KEYBOARD
		in.Ki.WScan = uint16(r)
		in.Ki.DwFlags = win.KEYEVENTF_UNICODE
		if win.SendInput(1, unsafe.Pointer(&in), int32(unsafe.Sizeof(in))) == 0 {
			return fmt.Errorf("键入字符 %q 失败", string(r))
		}
		in.Ki.DwFlags = win.KEYEVENTF_UNICODE | win.KEYEVENTF_KEYUP
		win.SendInput(1, unsafe.Pointer(&in), int32(unsafe.Sizeof(in)))
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

// typeAndSend 原子化：键入文本后立即回车发送（中间不做任何窗口/焦点操作），用于聊天等场景
func typeAndSend(text string) (string, error) {
	if err := typeText(text); err != nil {
		return "", err
	}
	time.Sleep(60 * time.Millisecond)
	if err := pressKey("enter"); err != nil {
		return "", err
	}
	return "已键入并回车发送: " + text, nil
}

// vkMap 常用键名 → 虚拟键码
var vkMap = map[string]uint16{
	"enter": 0x0D, "return": 0x0D, "tab": 0x09, "esc": 0x1B, "escape": 0x1B,
	"backspace": 0x08, "delete": 0x2E, "del": 0x2E, "space": 0x20,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	"home": 0x24, "end": 0x23, "pageup": 0x21, "pagedown": 0x22,
	"ctrl": 0x11, "control": 0x11, "shift": 0x10, "alt": 0x12, "menu": 0x12,
	"win": 0x5B, "cmd": 0x5B, "meta": 0x5B,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75,
	"f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
}

// parseKeyCombo 将键/组合键字符串解析为虚拟键码序列（按下顺序）
func parseKeyCombo(key string) ([]uint16, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(key)), "+")
	if len(parts) == 0 {
		return nil, fmt.Errorf("键名为空")
	}
	var codes []uint16
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if vk, ok := vkMap[p]; ok {
			codes = append(codes, vk)
			continue
		}
		// 单字符：字母取大写 ASCII 码，数字取对应 VK
		r := []rune(p)
		if len(r) == 1 {
			c := r[0]
			if c >= 'a' && c <= 'z' {
				codes = append(codes, uint16(c-'a'+'A'))
				continue
			}
			if c >= '0' && c <= '9' {
				codes = append(codes, uint16(c))
				continue
			}
		}
		return nil, fmt.Errorf("不支持的键名「%s」", p)
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("无法解析键「%s」", key)
	}
	return codes, nil
}

// parseKeyWithHold 解析按住语义前缀（short:/long:/hold:），返回纯组合键与按住毫秒数
func parseKeyWithHold(key string) (string, int, error) {
	k := strings.TrimSpace(key)
	lower := strings.ToLower(k)
	holdMs := 0
	for _, pre := range []struct {
		tag string
		ms  int
	}{{"short:", 1000}, {"long:", 10000}, {"hold:", 10000}} {
		if strings.HasPrefix(lower, pre.tag) {
			holdMs = pre.ms
			k = strings.TrimSpace(k[len(pre.tag):])
			break
		}
	}
	if k == "" {
		return "", 0, fmt.Errorf("键名为空")
	}
	return k, holdMs, nil
}

// pressKey 按下单个键或组合键，支持按住语义（short: 短按1秒 / long: 或 hold: 长按10秒）
func pressKey(key string) error {
	combo, holdMs, err := parseKeyWithHold(key)
	if err != nil {
		return err
	}
	codes, err := parseKeyCombo(combo)
	if err != nil {
		return err
	}
	// 全部按下
	for _, vk := range codes {
		keybdEvent(vk)
		time.Sleep(10 * time.Millisecond)
	}
	// 按住语义：保持按下状态一段时间
	if holdMs > 0 {
		time.Sleep(time.Duration(holdMs) * time.Millisecond)
	}
	// 逆序释放
	for i := len(codes) - 1; i >= 0; i-- {
		procKeybdEvent.Call(uintptr(codes[i]), 0, keyeventfKeyup, 0)
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// scrollWheelAt 先把光标移到屏幕绝对坐标，再在该位置滚动滚轮。
// 用于 scroll_wheel 工具：保证滚轮事件落在目标区域（避免在当前光标位置误滚）。
func scrollWheelAt(x, y int, direction string, amount int) error {
	if err := mouseMove(x, y); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	if amount <= 0 {
		amount = 3
	}
	delta := uintptr(amount * wheelDelta)
	if strings.EqualFold(direction, "down") {
		delta = uintptr(-int(amount) * wheelDelta)
	}
	mouseEvent(moeEventfWheel, 0, 0, delta)
	return nil
}

// ==== 屏幕捕获与坐标网格标注 ====

// captureAnnotatedScreen PNG 编码叠加坐标网格后的「焦点窗口」截图。
// 采用屏幕区域截图（基于屏幕 DC，能正确截到硬件加速窗口内容），只截前台窗口所在区域，体积远小于全屏截图。
func captureAnnotatedScreen() ([]byte, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return nil, fmt.Errorf("无前台窗口")
	}
	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, fmt.Errorf("获取窗口区域失败")
	}
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}
	// 先标注坐标网格，再整体缩放到最大边 visionMaxDim。
	// 缩放复用 image_processor 自带的高质量等比例缩放 ResizeToFit（Lanczos）：
	// 比手动最近邻 downscaleImage 更清晰，保证缩放后坐标网格刻度/数字仍准确可读、无失真。
	img, err := imageproc.CaptureScreenRegionRGBA(int(rect.Left), int(rect.Top), w, h)
	if err != nil {
		return nil, fmt.Errorf("窗口区域截图失败: %v", err)
	}
	annotateScreen(img)
	img = imageproc.ResizeToFit(img, visionMaxDim, visionMaxDim)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// captureWindowThumb 截取当前前台窗口的缩略图（等比缩放到 maxSide 内），用于操作前后画面差异检测。
func captureWindowThumb() (*image.RGBA, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return nil, fmt.Errorf("无前台窗口")
	}
	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return nil, fmt.Errorf("获取窗口区域失败")
	}
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}
	img, err := imageproc.CaptureScreenRegionRGBA(int(rect.Left), int(rect.Top), w, h)
	if err != nil {
		return nil, fmt.Errorf("窗口区域截图失败: %v", err)
	}
	return imageproc.ResizeToFit(img, diffThumbDim, diffThumbDim), nil
}

// diffWindowThumb 比较操作前/后两张窗口缩略图，返回是否发生变化及变化像素占比。
// 这是「图算数」的关键：用实际画面差异客观判定操作是否生效，而非依赖模型自述。
func diffWindowThumb(before, after *image.RGBA) (changed bool, changedRatio float64, detail string) {
	if before == nil || after == nil {
		return false, 0, "无操作前后对照图"
	}
	b := before.Bounds()
	a := after.Bounds()
	if b.Dx() != a.Dx() || b.Dy() != a.Dy() {
		return true, 1, "前后窗口尺寸不同（窗口可能已变化）"
	}
	total := b.Dx() * b.Dy()
	if total == 0 {
		return false, 0, "窗口为空"
	}
	changedPix := 0
	// 只统计有意义的色差（阈值 12，抗噪），避免光标/闪烁等微小扰动引起误报
	thr := 3 * 12 * 12
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c1 := before.RGBAAt(x, y)
			c2 := after.RGBAAt(x, y)
			dr := int(c1.R) - int(c2.R)
			dg := int(c1.G) - int(c2.G)
			db := int(c1.B) - int(c2.B)
			if dr*dr+dg*dg+db*db > thr {
				changedPix++
			}
		}
	}
	ratio := float64(changedPix) / float64(total)
	// 轻微变化（<0.3%）视为无实质变化，避免光标/时钟等微小扰动干扰
	if ratio < 0.003 {
		return false, ratio, "画面几乎无变化"
	}
	return true, ratio, fmt.Sprintf("画面发生变化（变化像素占比 %.1f%%）", ratio*100)
}

// windowCoordsToScreen 将前台窗口内相对坐标转换为屏幕绝对坐标（点击用）。
// 截图标注的网格数字即窗口内坐标；点击时加上窗口左上角屏幕偏移得到真实屏幕坐标。
func windowCoordsToScreen(wx, wy int) (int, int, error) {
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return 0, 0, fmt.Errorf("无前台窗口")
	}
	var rect win.RECT
	if !win.GetWindowRect(hwnd, &rect) {
		return 0, 0, fmt.Errorf("获取窗口区域失败")
	}
	return int(rect.Left) + wx, int(rect.Top) + wy, nil
}

// ==== 网格与坐标数字标注（纯位图字体，零外部依赖） ====

var (
	gridColor  = color.RGBA{0, 200, 220, 255}
	labelBg    = color.RGBA{15, 15, 25, 235}
	labelColor = color.RGBA{60, 255, 160, 255}
)

// digitPatterns 3×5 数字点阵（0-9）
var digitPatterns = map[byte][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "010", "010", "010"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
}

// setPixel 带边界检查地写入像素
func setPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	o := y*img.Stride + x*4
	img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c.R, c.G, c.B, c.A
}

// fillRect 填充矩形
func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			setPixel(img, i, j, c)
		}
	}
}

// drawNumber 在位点 (x, y) 以 scale 倍率绘制数字字符串（含背景衬底）
func drawNumber(img *image.RGBA, x, y int, num string, scale int) {
	charW := 3 * scale
	charH := 5 * scale
	totalW := len(num)*charW + (len(num)-1)*scale
	// 背景衬底提升可读性
	fillRect(img, x-2, y-2, totalW+4, charH+4, labelBg)
	cx := x
	for i := 0; i < len(num); i++ {
		p, ok := digitPatterns[num[i]]
		if !ok {
			cx += charW + scale
			continue
		}
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if p[row][col] == '1' {
					fillRect(img, cx+col*scale, y+row*scale, scale, scale, labelColor)
				}
			}
		}
		cx += charW + scale
	}
}

// annotateScreen 在截图上叠加坐标网格与刻度数字（原生像素坐标，与点击一致）
func annotateScreen(img *image.RGBA) {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	// 网格线
	for x := gridStep; x < w; x += gridStep {
		for y := 0; y < h; y++ {
			setPixel(img, x, y, gridColor)
		}
	}
	for y := gridStep; y < h; y += gridStep {
		for x := 0; x < w; x++ {
			setPixel(img, x, y, gridColor)
		}
	}
	// 顶部 X 刻度与左侧 Y 刻度
	for x := 0; x <= w; x += gridStep {
		drawNumber(img, x+2, 2, fmt.Sprintf("%d", x), 2)
	}
	for y := 0; y <= h; y += gridStep {
		drawNumber(img, 2, y+2, fmt.Sprintf("%d", y), 2)
	}
}
