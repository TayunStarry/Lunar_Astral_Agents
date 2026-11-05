package console

// view_update_body.go 更新包屏的菜单/版本/输入/运行/交接视图渲染。

import (
	"fmt"
	"strings"
)

// viewUpdate 更新包屏渲染
func (m appModel) viewUpdate() string {
	f := m.frameFor("更新包", updateContext(m.update), updateKeys(m.update))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.update.mode == updateHandover:
		body = updateHandoverBody(m, w)
		f.status = warnStyle.Render(fmt.Sprintf("%d 秒后自动退出", m.update.handoverLeft))
	case m.update.running:
		body = updateRunningBody(m, w, h)
	case m.update.mode == updateInputTag || m.update.mode == updateInputHash || m.update.mode == updateInputToken:
		body = m.updateInputBody(w)
	case m.update.mode == updateVersions:
		body = m.updateVersionsBody(w, h)
		f.status = dimStyle.Render(posText(m.update.versionIx, len(m.update.versions), "个版本"))
	default:
		body = m.updateMenuBody(w, h)
	}
	body = withErrStrip(body, m.update.err, w)
	return f.render(body)
}

// updateHandoverBody 交接画面：把「接下来会发生什么、去哪看」讲清楚。
//
// 用户视角的真相：点了「拉取最新」之后控制台会消失（主程序 exe 被锁，必须退出才能覆盖），
// 若不给这段说明，屏幕上就只是「黑了」，无从判断是成功还是失败。
func updateHandoverBody(_ appModel, w int) []string {
	note := pendingHandover
	keyW := 12
	body := []string{
		"",
		"  " + okStyle.Render("✓ 更新包已下载，主程序更新已交给更新器接续"),
		"",
		"  " + labelStyle.Render("接下来会自动完成（无需任何操作）："),
		"    " + dimStyle.Render("① 控制台退出 → ② 释放主程序文件占用 → ③ 解压并覆盖 → ④ 原样重启程序"),
		"",
		sectionRule("本次更新", w),
		"",
	}
	if note != nil {
		body = append(body,
			kvRow("更新包", note.PackFile, keyW),
			kvRow("覆盖目标", note.Root, keyW),
			kvRow("主程序", note.Program, keyW),
			kvRow("过程日志", note.LogPath, keyW),
		)
		if len(note.Files) > 0 {
			body = append(body, kvRow("纳入文件", strings.Join(note.Files, "、"), keyW))
		}
	} else {
		body = append(body, dimStyle.Render("  （交接信息不可用，详见过程日志）"))
	}
	body = append(body,
		"",
		"  "+warnStyle.Render("请勿关闭本终端窗口：")+dimStyle.Render("更新器是它的子进程，关窗会连带终止覆盖过程。"),
		"  "+dimStyle.Render("程序会自动重启；若长时间未重启，请查看上面的过程日志。"),
	)
	return body
}

// updateRunningBody 任务进行中
func updateRunningBody(m appModel, w, h int) []string {
	lines := []string{
		"",
		"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render(m.update.runningOp+"…"),
		"",
	}
	if m.update.progress > 0 {
		lines = append(lines, "  "+progressBar(m.update.progress, minInt(w-8, 60)))
	} else {
		lines = append(lines, "  "+dimStyle.Render("准备中：正在与 Gitee 建立通道"))
	}
	lines = append(lines, "", "  "+dimStyle.Render("详细过程见「原始日志」"))
	return fitLines(lines, h)
}

// updateMenuBody 操作菜单（可选中，数字键直达）
func (m appModel) updateMenuBody(w, h int) []string {
	head := []string{
		sectionRule("更新包操作", w),
		dimStyle.Render("  上下键选择后回车执行，也可直接按数字键"),
		"",
	}
	idx := clampInt(m.update.menuIdx, 0, len(updateMenuEntries)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(updateMenuEntries))
	for i, ent := range updateMenuEntries {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		title := labelStyle.Render("[" + ent.key + "] " + ent.title)
		if sel {
			title = selectStyle.Render("[" + ent.key + "] " + ent.title)
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, ""),
			newRow(rowW, sel).L("     ", dimStyle.Render(truncTo(ent.desc, maxInt(8, rowW-6)))).String(),
		}
	}
	lines, _ := m.update.menuPl.layoutView(blocks, idx, w, maxInt(1, h-len(head)-2))
	body := append([]string{}, head...)
	body = append(body, lines...)
	body = append(body, "", "  "+dimStyle.Render("更新包会自动关闭本程序、备份覆盖并重启"))
	return fitLines(body, h)
}

// updateInputBody 输入态
func (m appModel) updateInputBody(w int) []string {
	var prompt, hint string
	switch m.update.mode {
	case updateInputTag:
		prompt = "输入要拉取的版本 tag（留空回车返回）："
		hint = "例如 v2026.10.14；对应 Gitee Release 的 tag 名"
	case updateInputHash:
		prompt = "输入要拉取的 commit 哈希（留空回车返回）："
		hint = "支持短哈希前缀；需该提交已发布更新包（按 tag 匹配）"
	default:
		prompt = "输入 Gitee 私人令牌（发布需 Release 读写权限，留空回车取消）："
		hint = "config/release.json 已保存令牌时自动预填，可直接回车发布；令牌只用于本次请求"
	}
	boxW := minInt(w, 78)
	body := []string{
		"",
		"  " + labelStyle.Render(prompt),
		"",
	}
	body = append(body, formBox([]string{"", "  " + m.update.input.View(), ""}, boxW)...)
	body = append(body, "", "  "+dimStyle.Render(hint))
	return body
}

// updateVersionsBody 版本列表（定高窗口滚动，可直接选中拉取）
func (m appModel) updateVersionsBody(w, h int) []string {
	head := []string{
		sectionRule("仓库已发布版本（仅更新包通道）", w),
		dimStyle.Render("  回车按该 tag 拉取更新包；同仓库的历史市场发布已自动排除"),
		"",
	}
	if len(m.update.versions) == 0 {
		body := append([]string{}, head...)
		body = append(body, dimStyle.Render("  仓库暂无发布版本"))
		return fitLines(body, h)
	}

	idx := clampInt(m.update.versionIx, 0, len(m.update.versions)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(m.update.versions))
	for i, v := range m.update.versions {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		title := labelStyle.Render("◆ " + dashIfEmpty(v.TagName))
		right := dimStyle.Render(dashIfEmpty(v.Name))
		if sel {
			title = selectStyle.Render("◆ " + dashIfEmpty(v.TagName))
			right = keyStyle.Render(dashIfEmpty(v.Name))
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, right),
			newRow(rowW, sel).L("    ", dimStyle.Render(truncTo(firstLine(v.Body), maxInt(8, rowW-6)))).String(),
		}
	}

	lines, _ := m.update.versionPl.layoutView(blocks, idx, w, maxInt(1, h-len(head)))
	body := append([]string{}, head...)
	body = append(body, lines...)
	return fitLines(body, h)
}
