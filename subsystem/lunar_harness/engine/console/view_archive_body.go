package console

// view_archive_body.go 打包归档屏的列表/详情/运行/输入视图渲染。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/archive"
)

// viewArchive 打包屏渲染
func (m appModel) viewArchive() string {
	f := m.frameFor("打包归档", archiveContext(m.archive), archiveKeys(m.archive))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.archive.running:
		body = archiveRunningBody(m, w, h)
	case m.archive.inputMode:
		body = m.archiveInputBody(w)
	case m.archive.loaded != nil:
		body = m.archiveDetailBody(w, h)
	default:
		body = m.archiveListBody(w, h)
		f.status = dimStyle.Render(posText(m.archive.cfgIdx, len(m.archive.configs), "个配置"))
	}
	body = withErrStrip(body, m.archive.err, w)
	return f.render(body)
}

// archiveRunningBody 打包进行中
func archiveRunningBody(m appModel, w, h int) []string {
	lines := []string{
		"",
		"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render("正在打包归档…"),
		"",
	}
	if m.archive.progress > 0 {
		lines = append(lines, "  "+progressBar(m.archive.progress, minInt(w-8, 60)))
	} else {
		lines = append(lines, "  "+dimStyle.Render("准备中：正在扫描文件清单"))
	}
	lines = append(lines, "",
		"  "+dimStyle.Render("配置: ")+valueStyle.Render(m.archive.loadedPath),
		"  "+dimStyle.Render("详细过程见「原始日志」"))
	return fitLines(lines, h)
}

// archiveInputBody 自定义配置文件路径输入
func (m appModel) archiveInputBody(w int) []string {
	boxW := minInt(w, 78)
	body := []string{
		"",
		"  " + labelStyle.Render("输入打包配置文件路径（留空回车返回列表）："),
		"",
	}
	body = append(body, formBox([]string{"", "  " + m.archive.input.View(), ""}, boxW)...)
	body = append(body,
		"",
		"  "+dimStyle.Render("默认配置：")+valueStyle.Render(archive.DefaultConfigPath),
		"  "+dimStyle.Render("也可放在 data/*.json，重新进入本屏即可自动发现"))
	return body
}

// archiveListBody 候选配置列表（定高窗口滚动）
func (m appModel) archiveListBody(w, h int) []string {
	head := []string{
		sectionRule("候选打包配置", w),
		dimStyle.Render("  回车加载配置并进入确认；按 [p] 可手动指定配置文件路径"),
		"",
	}
	if len(m.archive.configs) == 0 {
		body := append([]string{}, head...)
		body = append(body,
			dimStyle.Render("  未找到打包配置（默认 "+archive.DefaultConfigPath+"，或 data/*.json）"),
			"",
			dimStyle.Render("  将配置文件放入上述任一位置后重新进入本屏，或按 [p] 输入其他路径"))
		return fitLines(body, h)
	}

	idx := clampInt(m.archive.cfgIdx, 0, len(m.archive.configs)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(m.archive.configs))
	for i, path := range m.archive.configs {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		name := path
		tag := ""
		if path == archive.DefaultConfigPath {
			tag = "默认"
		}
		title := labelStyle.Render("◆ " + name)
		if sel {
			title = selectStyle.Render("◆ " + name)
		}
		blocks[i] = []string{listRow(rowW, sel, cursor, title, dimStyle.Render(tag))}
	}

	lines, _ := m.archive.list.layoutView(blocks, idx, w, maxInt(1, h-len(head)))
	body := append([]string{}, head...)
	body = append(body, lines...)
	return fitLines(body, h)
}

// archiveDetailBody 已加载配置确认态：键值卡片
func (m appModel) archiveDetailBody(w, h int) []string {
	cfg := m.archive.loaded
	if cfg == nil {
		return blankLines(h, w)
	}
	keyW := 10
	rows := []string{
		sectionRule("已加载配置", w),
		"",
		kvRow("配置文件", m.archive.loadedPath, keyW),
		kvRow("输出路径", cfg.Output, keyW),
		kvRow("分卷大小", fmt.Sprintf("%d MB", cfg.Size), keyW),
		kvRow("压缩等级", fmt.Sprintf("%d", cfg.Level), keyW),
		kvRow("打包项数", fmt.Sprintf("%d", len(cfg.Import)), keyW),
	}
	if len(cfg.Exclude) > 0 {
		rows = append(rows, kvRow("排除项", strings.Join(cfg.Exclude, "  "), keyW))
	}
	if len(cfg.Import) > 0 {
		rows = append(rows, "", sectionRule("打包来源", w), "")
		room := maxInt(1, h-len(rows)-2)
		for _, item := range cfg.Import {
			if room <= 0 {
				break
			}
			rows = append(rows, "  "+labelStyle.Render(truncTo(item, maxInt(8, w-4))))
			room--
		}
	}
	rows = append(rows, "", "  "+dimStyle.Render("按 Enter 开始打包；Esc 返回配置列表"))
	return fitLines(rows, h)
}
