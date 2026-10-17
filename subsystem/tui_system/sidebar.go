package TuiSystem

// ==== 侧边导航栏（nav rail）====
//
// 为什么要有它：早期界面是「主菜单 → 功能屏 → Esc 返回」两级结构，用户在功能屏里
// 看不到自己处在整个程序的什么位置，也不知道还有哪些屏，只能记住快捷键或退回主菜单。
// 主流终端程序（lazygit / k9s / yazi）都用常驻侧栏解决这件事：**位置与入口同时可见**。
//
// 两条刻意的取舍：
//   - 只在 **非主菜单屏**且足够宽时出现：主菜单本身就是导航，再放侧栏是重复；
//     窄终端（<navShowFrom）收起侧栏，避免把内容区挤到不可用。
//   - 条目一律来自 menuItems（快捷键、标题、分组、目标屏都在那张表里），
//     侧栏因此不会与主菜单、帮助页漂移。
//
// 侧栏不画自己的外框：它嵌在外框里，右侧用一条竖线分隔（见 layout.go 的 frame.render）。

const (
	// navShowFrom 显示侧栏所需的最小终端列数：再窄就只剩内容区了
	navShowFrom = 96
	// navWidthMin / navWidthMax 侧栏宽度的收敛区间
	navWidthMin = 18
	navWidthMax = 26
	// navMinHeight 侧栏至少需要的高度（分组标题 + 若干条目）
	navMinHeight = 10
)

// navRow 侧栏的一行：idx>=0 指向 menuItems，idx<0 表示分组标题（group 有效）
type navRow struct {
	idx   int
	group string
}

// navVisible 当前是否显示侧栏
func (m appModel) navVisible() bool {
	if m.help || m.screen == screenMenu {
		return false
	}
	if m.width < navShowFrom || m.height < navMinHeight {
		return false
	}
	return len(menuItems) > 0
}

// navWidth 侧栏宽度：按最长条目名与快捷键标记算，收敛在 [navWidthMin, navWidthMax]
func navWidth() int {
	labelW, keyW := 0, 0
	for _, it := range menuItems {
		labelW = maxInt(labelW, dispWidth(it.title))
		keyW = maxInt(keyW, dispWidth("["+it.key+"]"))
	}
	// 光标 2 + 名称 + 间隔 2 + 快捷键标记 + 右侧留白 2（贴住分隔线会显得挤）
	return clampInt(2+labelW+2+keyW+2, navWidthMin, navWidthMax)
}

// navRows 侧栏行表：组首插入分组标题（顺序 = menuItems 顺序）
func navRows() []navRow {
	rows := make([]navRow, 0, len(menuItems)+4)
	for i, it := range menuItems {
		if i == 0 || menuItems[i-1].group != it.group {
			rows = append(rows, navRow{idx: -1, group: it.group})
		}
		rows = append(rows, navRow{idx: i})
	}
	return rows
}

// menuIndexOf 目标屏在主菜单表里的下标（未命中返回 -1）
func menuIndexOf(id screenID) int {
	for i, it := range menuItems {
		if it.screen == id && id != screenMenu {
			return i
		}
	}
	return -1
}

// screenGroup 当前屏所属分组（面包屑用；主菜单无分组）
func (m appModel) screenGroup() string {
	if i := menuIndexOf(m.screen); i >= 0 {
		return menuItems[i].group
	}
	return ""
}

// navLines 渲染恰好 h 行的侧栏内容。
//
// 高度不足时不是简单截断，而是**围绕当前屏开窗**：当前位置永远可见——
// 侧栏若把「我在哪」滚掉了，它的存在意义就没了。
func (m appModel) navLines(w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	head := newRow(w, false).
		L(" ", faintStyle.Render(truncTo("导航", w-2))).
		String()

	out := make([]string, 0, h)
	out = append(out, head)
	avail := h - len(out)
	if avail <= 0 {
		return fitLines(out, h)
	}

	rows := navRows()
	cur := 0
	for i, r := range rows {
		if r.idx >= 0 && menuIndexOf(m.screen) == r.idx {
			cur = i
			break
		}
	}
	top := 0
	if len(rows) > avail {
		top = clampInt(cur-avail/2, 0, len(rows)-avail)
	}
	for i := top; i < len(rows) && i < top+avail; i++ {
		out = append(out, m.navRowLine(w, rows[i]))
	}
	return fitLines(out, h)
}

// navRowLine 单行侧栏：分组标题 / 普通条目 / 当前屏（整行反白）
func (m appModel) navRowLine(w int, row navRow) string {
	if row.idx < 0 {
		return newRow(w, false).
			L(" ", navGroupStyle.Render(truncTo(row.group, maxInt(4, w-2)))).
			String()
	}
	it := menuItems[row.idx]
	active := it.screen == m.screen && it.screen != screenMenu

	cursor := "  "
	label := navItemStyle.Render(it.title)
	key := navKeyStyle.Render("[" + it.key + "]")
	if active {
		// 选中行交给 rowBuilder 逐片段上底色：名称用反白前景，快捷键标记保留可读灰
		cursor = cursorStyle.Render("▸ ")
		label = it.title
		key = selDimStyle.Render("[" + it.key + "]")
	}
	return newRow(w, active).L(cursor, label).R(key).String()
}

// navigableScreens 可用 [ / ] 顺序切换的屏幕（= 侧栏顺序，末项「退出」不在其中）
func navigableScreens() []screenID {
	out := make([]screenID, 0, len(menuItems))
	for _, it := range menuItems {
		if it.screen == screenMenu {
			continue // 退出程序的哨兵
		}
		out = append(out, it.screen)
	}
	return out
}
