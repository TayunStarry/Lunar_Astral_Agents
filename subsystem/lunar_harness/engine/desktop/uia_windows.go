//go:build windows

package desktop

// ==== UI Automation 读写 ====
// 负责 UIA 客户端初始化、UI 树遍历、元素查找，以及点击/写入操作。

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	uia "github.com/auuunya/go-element"
	"github.com/lxn/win"
)

// uiaInit 一次性初始化 COM 与 UIAutomation 客户端实例。
func uiaInit() error {
	var rerr error
	uiaOnce.Do(func() {
		if err := uia.CoInitialize(); err != nil {
			rerr = err
			return
		}
		instance, err := uia.CreateInstance(uia.CLSID_CUIAutomation, uia.IID_IUIAutomation, uia.CLSCTX_INPROC_SERVER|uia.CLSCTX_LOCAL_SERVER)
		if err != nil || instance == nil {
			rerr = fmt.Errorf("创建 UIAutomation 实例失败: %v", err)
			return
		}
		uiaClient = uia.NewIUIAutomation(uia.NewIUnKnown(instance))
	})
	return rerr
}

// uiaRoot 返回当前前台窗口作为 UIA 树根元素。
func uiaRoot() (*uia.Element, error) {
	if err := uiaInit(); err != nil {
		return nil, err
	}
	hwnd := win.GetForegroundWindow()
	if hwnd == 0 {
		return nil, fmt.Errorf("无前台窗口")
	}
	ie, err := uia.ElementFromHandle(uiaClient, uintptr(hwnd))
	if err != nil || ie == nil {
		return nil, fmt.Errorf("UIA 无法读取当前窗口: %v", err)
	}
	root := &uia.Element{UIAutoElement: ie}
	root.Populate(false)
	return root, nil
}

// uiaWalk 自实现 DFS 遍历 UIA 元素树并对每个元素调用 fn。
func uiaWalk(root *uia.Element, fn func(e *uia.Element, depth int)) {
	cond := uia.CreateTrueCondition(uiaClient)
	if root == nil {
		return
	}
	var rec func(ie *uia.IUIAutomationElement, depth int)
	rec = func(ie *uia.IUIAutomationElement, depth int) {
		if ie == nil {
			return
		}
		e := &uia.Element{UIAutoElement: ie}
		e.Populate(false)
		fn(e, depth)
		if cond == nil {
			return
		}
		arr, err := uia.FindAll(ie, cond)
		if err != nil || arr == nil {
			return
		}
		if n := uia.Get_Length(arr); n > 0 {
			for i := int32(0); i < n; i++ {
				ch, _ := uia.GetElement(arr, i)
				if ch != nil {
					rec(ch, depth+1)
				}
			}
		}
	}
	rec(root.UIAutoElement, 0)
}

// uiaCtrlName 将 UIA 控件类型 ID 映射为中文名称。
func uiaCtrlName(ct uia.ControlTypeId) string {
	switch ct {
	case uia.UIA_ButtonControlTypeId:
		return "按钮"
	case uia.UIA_EditControlTypeId:
		return "输入框"
	case uia.UIA_CheckBoxControlTypeId:
		return "复选框"
	case uia.UIA_ComboBoxControlTypeId:
		return "下拉框"
	case uia.UIA_DocumentControlTypeId:
		return "文档"
	case uia.UIA_ListItemControlTypeId:
		return "列表项"
	case uia.UIA_ListControlTypeId:
		return "列表"
	case uia.UIA_MenuItemControlTypeId:
		return "菜单项"
	case uia.UIA_TabControlTypeId:
		return "标签页"
	case uia.UIA_TabItemControlTypeId:
		return "标签项"
	case uia.UIA_PaneControlTypeId:
		return "面板"
	case uia.UIA_WindowControlTypeId:
		return "窗口"
	case uia.UIA_TextControlTypeId:
		return "文本"
	case uia.UIA_TreeControlTypeId:
		return "树"
	case uia.UIA_TreeItemControlTypeId:
		return "树项"
	case uia.UIA_RadioButtonControlTypeId:
		return "单选钮"
	}
	return "?"
}

// uiaBrief 生成元素的单行摘要（名称/类型/id/类名/禁用状态）。
func uiaBrief(e *uia.Element) string {
	n := strings.TrimSpace(e.CurrentName)
	if n == "" {
		n = "（无名）"
	}
	parts := []string{fmt.Sprintf("[%s]", uiaCtrlName(e.CurrentControlType))}
	if id := strings.TrimSpace(e.CurrentAutomationId); id != "" {
		parts = append(parts, "id="+id)
	}
	if cls := strings.TrimSpace(e.CurrentClassName); cls != "" {
		parts = append(parts, "class="+cls)
	}
	if e.CurrentIsEnabled == 0 {
		parts = append(parts, "禁用")
	}
	return n + " " + strings.Join(parts, " ")
}

// UIADump 输出前台窗口完整 UI 树文本（带缩进层级）。
func UIADump() (string, bool, error) {
	root, err := uiaRoot()
	if err != nil {
		return "", false, err
	}
	var sb strings.Builder
	sb.WriteString("【当前前台窗口 UI 树】\n")
	uiaWalk(root, func(e *uia.Element, d int) {
		sb.WriteString(strings.Repeat("  ", d))
		sb.WriteString(uiaBrief(e))
		sb.WriteString("\n")
	})
	return sb.String(), true, nil
}

// UIAFind 按名称子串与控件类型在 UI 树中查找元素，返回带序号的匹配列表。
func UIAFind(name, controlType string) ([]string, bool, error) {
	root, err := uiaRoot()
	if err != nil {
		return nil, false, err
	}
	var matches []*uia.Element
	lowerName := strings.ToLower(name)
	uiaWalk(root, func(e *uia.Element, _ int) {
		ctName := uiaCtrlName(e.CurrentControlType)
		nameOk := name == "" || strings.Contains(strings.ToLower(e.CurrentName), lowerName)
		typeOk := controlType == "" || ctName == controlType || strings.EqualFold(e.CurrentClassName, controlType)
		if nameOk && typeOk {
			matches = append(matches, e)
		}
	})
	lines := []string{}
	for i, e := range matches {
		lines = append(lines, fmt.Sprintf("%d. %s", i, uiaBrief(e)))
	}
	return lines, len(lines) > 0, nil
}

// uiaCenter 计算元素边框的中心屏幕坐标。
func uiaCenter(e *uia.Element) (x, y int, ok bool) {
	if e == nil {
		return 0, 0, false
	}
	e.BoundingRectangle()
	if e.CurrentBoundingRectangle == nil {
		return 0, 0, false
	}
	r := e.CurrentBoundingRectangle
	return (int(r.Left) + int(r.Right)) / 2, (int(r.Top) + int(r.Bottom)) / 2, true
}

// uiaResolve 将目标（名称子串或序号）+ 控件类型解析为具体元素并返回其摘要。
func uiaResolve(target, controlType string) (*uia.Element, string, error) {
	root, err := uiaRoot()
	if err != nil {
		return nil, "", err
	}
	name := strings.TrimSpace(target)
	idx := -1
	if name != "" {
		if n, e := strconv.Atoi(name); e == nil && n >= 0 {
			idx = n
			name = ""
		}
	}
	var matches []*uia.Element
	lowerName := strings.ToLower(name)
	uiaWalk(root, func(e *uia.Element, _ int) {
		ctName := uiaCtrlName(e.CurrentControlType)
		nameOk := name == "" || strings.Contains(strings.ToLower(e.CurrentName), lowerName)
		typeOk := controlType == "" || ctName == controlType || strings.EqualFold(e.CurrentClassName, controlType)
		if nameOk && typeOk {
			matches = append(matches, e)
		}
	})
	if len(matches) == 0 {
		return nil, "", fmt.Errorf("UIA 未找到匹配元素（目标=%q）", target)
	}
	if idx >= 0 && idx < len(matches) {
		return matches[idx], uiaBrief(matches[idx]), nil
	}
	return matches[0], uiaBrief(matches[0]), nil
}

// uiaActivate 对元素执行激活（优先 Invoke/SelectionItem，失败则坐标兜底点击）。
func uiaActivate(e *uia.Element) (string, bool) {
	if p, err := e.GetInvokePattern(); err == nil && p != nil {
		if e2 := p.Invoke(); e2 == nil {
			return "通过 UIA Invoke 点击", true
		}
	}
	if p, err := e.GetSelectionItemPattern(); err == nil && p != nil {
		if e2 := p.Select(); e2 == nil {
			return "通过 UIA SelectionItem 选择", true
		}
	}
	if x, y, ok := uiaCenter(e); ok {
		if err := MouseClick(x, y, "", false); err == nil {
			return fmt.Sprintf("坐标兜底点击(%d,%d)", x, y), true
		}
	}
	return "", false
}

// uiaSetText 向元素写入文本（优先 Value 模式，失败则聚焦后键入）。
func uiaSetText(e *uia.Element, text string) (string, bool) {
	if p, err := e.GetValuePattern(); err == nil && p != nil {
		if e2 := p.SetValue(text); e2 == nil {
			return "通过 UIA Value 写入", true
		}
	}
	if x, y, ok := uiaCenter(e); ok {
		if err := MouseClick(x, y, "", false); err == nil {
			time.Sleep(80 * time.Millisecond)
			if err := TypeText(text); err == nil {
				return fmt.Sprintf("坐标聚焦(%d,%d)后键入", x, y), true
			}
		}
	}
	return "", false
}

// UIAClick 通过 UIA 查找并激活（点击）目标元素。
func UIAClick(target, controlType string) (string, bool, error) {
	e, brief, err := uiaResolve(target, controlType)
	if err != nil {
		return "", false, err
	}
	how, ok := uiaActivate(e)
	if !ok {
		return "", false, fmt.Errorf("UIA 激活失败: %s", brief)
	}
	return fmt.Sprintf("%s（%s）", brief, how), true, nil
}

// UIAInput 通过 UIA 查找输入框并写入文本，输入框类型不匹配时放宽类型约束重试。
func UIAInput(target, text string) (string, bool, error) {
	e, brief, err := uiaResolve(target, "输入框")
	if err != nil {
		// 放宽类型约束重试
		if e2, b2, e2err := uiaResolve(target, ""); e2err == nil {
			e, brief = e2, b2
		} else {
			return "", false, err
		}
	}
	how, ok := uiaSetText(e, text)
	if !ok {
		return "", false, fmt.Errorf("UIA 写入失败: %s", brief)
	}
	return fmt.Sprintf("%s（%s）", brief, how), true, nil
}
