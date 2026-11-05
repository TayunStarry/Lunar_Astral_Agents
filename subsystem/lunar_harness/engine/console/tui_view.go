package console

// tui_view.go 控制台 TUI 的渲染分发、屏元信息与主菜单视图。

// View 渲染分发
func (m appModel) View() string {
	if m.width == 0 {
		return "正在初始化界面…"
	}
	if m.help {
		return m.viewHelp()
	}

	switch m.screen {
	case screenMenu:
		return m.viewMenu()
	case screenAgent:
		return m.viewAgent()
	case screenStats:
		return m.viewStats()
	case screenLogs:
		return m.viewSystemLogs()
	case screenProxy:
		return m.viewProxy()
	case screenArchive:
		return m.viewArchive()
	case screenUpdate:
		return m.viewUpdate()
	case screenPorts:
		return m.viewPorts()
	case screenDump:
		return m.viewDump()
	case screenTools:
		return m.viewTools()
	}
	return ""
}

// screenMeta 当前屏的展示名、上下文与键位（同一份数据驱动标题栏、底栏与帮助页）
func (m appModel) screenMeta() (name, context string, keys []keyHint) {
	switch m.screen {
	case screenMenu:
		return "主菜单", "", menuKeys()
	case screenAgent:
		return "推理实况", "", agentKeys(m.agent)
	case screenStats:
		return "运行状态", "", engineKeys(m.engine)
	case screenLogs:
		return "系统日志 · " + logCategoryTitles[m.logCat], "", logsKeys()
	case screenProxy:
		return "HTTPS 代理", proxyContext(m.proxy), proxyKeys(m.proxy)
	case screenArchive:
		return "打包归档", archiveContext(m.archive), archiveKeys(m.archive)
	case screenUpdate:
		return "更新包", updateContext(m.update), updateKeys(m.update)
	case screenPorts:
		return "端口释放", "", portsKeys()
	case screenDump:
		return "数据导出", dumpContextLine(m.dump), dumpKeys(m.dump)
	case screenTools:
		return "工具列表", toolsContextLine(m.tools), toolsKeys()
	}
	return "未知", "", nil
}

// menuKeys 主菜单键位
func menuKeys() []keyHint {
	return []keyHint{
		kh("↑/↓", "选择"),
		kh("Enter/→", "进入"),
		kh("0-9 a", "直达"),
		kh("?", "帮助"),
		kh("q", "退出"),
	}
}

// dashIfEmpty 空快捷键在详情栏显示为破折号
func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// menuDetail 主菜单右侧详情：说明 + 元信息（宽屏两栏布局的右栏）
func (m appModel) menuDetail(idx, w, h int) []string {
	if idx < 0 || idx >= len(menuItems) || w <= 0 {
		return blankLines(h, w)
	}
	it := menuItems[idx]
	lines := []string{
		selectStyle.Render("✦ " + it.title),
		"",
		kvRow("快捷键", fitWidth(dashIfEmpty(it.key), 4), 8),
		kvRow("分组", it.group, 8),
		kvRow("入口", "Enter 或 ["+it.key+"]", 8),
		"",
		sectionRule("说明", w),
	}
	for _, l := range wrapPlain(it.desc, maxInt(8, w-2)) {
		lines = append(lines, "  "+labelStyle.Render(l))
	}
	return fitLines(lines, h)
}

// viewMenu 菜单屏：左侧分组列表 + 右侧详情（宽屏）。
// 列表经定高窗口渲染，条目再多也不会溢出终端。
func (m appModel) viewMenu() string {
	f := m.frameFor("主菜单", "", menuKeys())
	w, h := f.bodySize()

	// 宽屏两栏：左列表 + 竖分隔 + 右详情；窄屏退化为单栏（说明置于标题之下）
	twoPane := w >= 88
	listW := w
	detailW := 0
	if twoPane {
		listW = clampInt(w*45/100, 36, 50)
		detailW = w - listW - 4 // 竖分隔 3 列 + 右栏尾部留白 1 列
	}
	rowW := maxInt(8, listW-listGutter)

	blocks := make([][]string, len(menuItems))
	for i, it := range menuItems {
		var b []string
		if i == 0 || menuItems[i-1].group != it.group { // 组首插入分组标题
			b = append(b, sectionRule(it.group, rowW))
		}
		sel := i == m.menu
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		keyTag := dimStyle.Render("[" + it.key + "]")
		title := labelStyle.Render(it.title)
		if sel {
			keyTag = keyStyle.Render("[" + it.key + "]")
			title = selectStyle.Render(it.title)
		}
		b = append(b, listRow(rowW, sel, cursor, title, keyTag))
		if !twoPane {
			b = append(b, newRow(rowW, sel).
				L("     ", dimStyle.Render(truncTo(it.desc, maxInt(4, rowW-6)))).
				String())
		}
		blocks[i] = b
	}

	lines, info := m.menuPl.layoutView(blocks, m.menu, listW, h)
	body := lines
	if twoPane {
		detail := m.menuDetail(m.menu, detailW, h)
		body = make([]string, h)
		for i := 0; i < h; i++ {
			body[i] = lines[i] + ruleStyle.Render(" │ ") + detail[i] + " "
		}
	}

	f.status = dimStyle.Render(info.positionText() + " 项")
	return f.render(body)
}
