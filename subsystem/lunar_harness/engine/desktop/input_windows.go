//go:build windows

package desktop

// ==== 鼠标与键盘输入原语 ====
// 负责屏幕坐标换算、鼠标点击/按住/拖拽/滚动、键盘键入/按键。

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/lxn/win"
)

// mouseEvent 通过 mouse_event 发送一次底层鼠标事件。
func mouseEvent(flags, dx, dy, data uintptr) {
	procMouseEvent.Call(flags, dx, dy, data, 0)
}

// MouseClick 移动到指定屏幕坐标并执行单击/双击（可选右键/中键）。
func MouseClick(x, y int, button string, double bool) error {
	if !win.SetCursorPos(int32(x), int32(y)) {
		return fmt.Errorf("移动光标到 (%d,%d) 失败", x, y)
	}
	time.Sleep(60 * time.Millisecond)
	down, up := uintptr(mswLeftdown), uintptr(mswLeftup)
	switch strings.ToLower(button) {
	case "right":
		down, up = mswRightdown, mswRightup
	case "middle":
		down, up = mswMiddledown, mswMiddleup
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

// MouseButton 移动到指定屏幕坐标并按住指定按键一段时长后松开（支持右键/中键）。
func MouseButton(x, y int, button string, holdMs int) error {
	if !win.SetCursorPos(int32(x), int32(y)) {
		return fmt.Errorf("移动光标到 (%d,%d) 失败", x, y)
	}
	time.Sleep(60 * time.Millisecond)
	down, up := uintptr(mswLeftdown), uintptr(mswLeftup)
	switch strings.ToLower(button) {
	case "right":
		down, up = mswRightdown, mswRightup
	case "middle":
		down, up = mswMiddledown, mswMiddleup
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

// MouseDrag 在屏幕上按住左键从 (x1,y1) 经多步插值拖拽到 (x2,y2) 后松开。
func MouseDrag(x1, y1, x2, y2 int, steps int) error {
	if steps < 2 {
		steps = 2
	}
	if steps > 64 {
		steps = 64
	}
	if !win.SetCursorPos(int32(x1), int32(y1)) {
		return fmt.Errorf("移动光标失败")
	}
	time.Sleep(60 * time.Millisecond)
	mouseEvent(mswLeftdown, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)
	for i := 1; i <= steps; i++ {
		cx := x1 + (x2-x1)*i/steps
		cy := y1 + (y2-y1)*i/steps
		win.SetCursorPos(int32(cx), int32(cy))
		time.Sleep(12 * time.Millisecond)
	}
	mouseEvent(mswLeftup, 0, 0, 0)
	return nil
}

// WindowToScreen 将前台窗口内相对坐标换算为屏幕绝对坐标。
func WindowToScreen(wx, wy int) (int, int, error) {
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

// MoveMouse 仅移动光标到指定屏幕坐标，不点击。
func MoveMouse(x, y int) error {
	if !win.SetCursorPos(int32(x), int32(y)) {
		return fmt.Errorf("移动光标到 (%d,%d) 失败", x, y)
	}
	return nil
}

// ScrollWheel 在指定屏幕坐标滚动滚轮指定格数（方向 up/down）。
func ScrollWheel(x, y int, direction string, amount int) error {
	if !win.SetCursorPos(int32(x), int32(y)) {
		return fmt.Errorf("移动光标失败")
	}
	time.Sleep(60 * time.Millisecond)
	if amount <= 0 {
		amount = 3
	}
	delta := uintptr(amount * 120)
	if strings.EqualFold(direction, "down") {
		delta = uintptr(-int(amount) * 120)
	}
	mouseEvent(mswWheel, 0, 0, delta)
	return nil
}

// TypeText 在当前焦点窗口逐字符键入文本（支持中文，使用 UNICODE 输入法）。
func TypeText(text string) error {
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

// TypeAndSend 键入文本后回车发送（用于聊天/搜索等带输入的场景）。
func TypeAndSend(text string) (string, error) {
	if err := TypeText(text); err != nil {
		return "", err
	}
	time.Sleep(60 * time.Millisecond)
	if err := PressKey("enter"); err != nil {
		return "", err
	}
	return "已键入并回车发送: " + text, nil
}

// PressKey 模拟单键/组合键按键，支持短按/长按语义前缀。
func PressKey(key string) error {
	k := strings.TrimSpace(key)
	holdMs := 0
	combo := k
	lower := strings.ToLower(k)
	for _, pre := range []struct {
		tag string
		ms  int
	}{{"short:", 1000}, {"long:", 10000}, {"hold:", 10000}} {
		if strings.HasPrefix(lower, pre.tag) {
			holdMs = pre.ms
			combo = strings.TrimSpace(k[len(pre.tag):])
			break
		}
	}
	if combo == "" {
		return fmt.Errorf("键名为空")
	}
	parts := strings.Split(strings.ToLower(combo), "+")
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
		return fmt.Errorf("不支持的键名「%s」", p)
	}
	if len(codes) == 0 {
		return fmt.Errorf("无法解析键「%s」", key)
	}
	down := func(vk uint16) { procKeybdEvent.Call(uintptr(vk), 0, 0, 0) }
	up := func(vk uint16) { procKeybdEvent.Call(uintptr(vk), 0, mswKeyup, 0) }
	for _, vk := range codes {
		down(vk)
	}
	if holdMs > 0 {
		time.Sleep(time.Duration(holdMs) * time.Millisecond)
	} else {
		time.Sleep(10 * time.Millisecond)
	}
	for i := len(codes) - 1; i >= 0; i-- {
		up(codes[i])
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}
