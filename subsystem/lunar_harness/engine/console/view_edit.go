package console

// view_edit.go 「编辑显示」屏的交互与渲染（纯展示层：不接受运行 / 编辑入口）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// ==== 交互与渲染 ====

// editKeys 编辑显示屏键位：只读展示 + 运行入口（不提供编辑 / 时长设置）。
func editKeys() []keyHint {
	return []keyHint{
		kh("←/→", "切换文件"),
		kh("↑↓", "滚动"),
		kh("R", "运行"),
		kh("Esc", "返回"),
	}
}

// editRunMsg 一次「运行当前文件」的完成回执。
type editRunMsg struct {
	path    string
	summary string
	err     error
}

// updateEdit 「编辑显示」屏事件：左右切换文件，上下滚动，R 键运行当前文件。
func (m appModel) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch k {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = screenMenu
			return m, nil
		case "left", "h":
			if len(m.editView.files) > 0 {
				m.editView.cursor = listMove(m.editView.cursor, -1, len(m.editView.files), true)
				m.editView.offset = 0
			}
			return m, nil
		case "right", "l":
			if len(m.editView.files) > 0 {
				m.editView.cursor = listMove(m.editView.cursor, 1, len(m.editView.files), true)
				m.editView.offset = 0
			}
			return m, nil
		case "home", "g":
			m.editView.offset = 0
			return m, nil
		case "end", "G":
			m.editView.offset = 1 << 30
			return m, nil
		case "r", "R":
			return m.runCurrentDoc()
		}
		if next, _, ok := scrollKey(m.editView.offset, k, m.editView.lines, m.editBodyRows(), false); ok {
			m.editView.offset = next
			return m, nil
		}
	}
	return m, nil
}

// runCurrentDoc 运行当前选中的可运行文件（只提供运行入口，不设置 / 显示运行时长）。
func (m appModel) runCurrentDoc() (tea.Model, tea.Cmd) {
	file := m.editView.currentFile()
	if file == nil || m.editView.running {
		return m, nil
	}
	if !isRunnableDoc(file.Path) {
		m.setNotice("该文件类型不可运行（仅 .js / .html / .htm）", true)
		return m, nil
	}
	rel := file.Path
	m.editView.running = true
	m.setNotice("正在运行 "+rel+"…", false)
	return m, func() tea.Msg {
		summary, err := RunFile(rel)
		return editRunMsg{path: rel, summary: summary, err: err}
	}
}

// isRunnableDoc 判断文件类型是否有运行入口。
func isRunnableDoc(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".html", ".htm":
		return true
	}
	return false
}

// editBottomRows 底部元数据占用行数（文件名 1 行 + 元数据 1 行）。
func (m appModel) editBottomRows() int { return 2 }

// editBodyRows 主显示区可见行数。
func (m appModel) editBodyRows() int {
	_, h := m.frameFor("", "", nil).bodySize()
	return maxInt(1, h-m.editBottomRows())
}

// viewEdit 「编辑显示」屏渲染：主内容区（带行号）+ 当前文件元数据 + 状态。
func (m appModel) viewEdit() string {
	f := m.frameFor("编辑显示", "documents · 展示与运行", editKeys())
	w, h := f.bodySize()

	files := m.editView.files
	if len(files) == 0 {
		body := emptyState("文档库为空", "local_data/documents 下还没有文件。",
			"AI 写入文件后，这里会实时列出并可左右切换查看", w, maxInt(1, h-2))
		body = append(body, dimStyle.Render("  文件 0 个"))
		return f.render(body)
	}

	mainH := m.editBodyRows()
	lines := m.editView.visibleLines(w, mainH)
	maxOff := maxInt(0, len(lines)-mainH)
	offset := clampInt(m.editView.offset, 0, maxOff)
	m.editView.offset = offset
	end := minInt(offset+mainH, len(lines))

	body := make([]string, 0, h)
	body = append(body, fitLines(lines[offset:end], mainH)...)
	body = append(body, m.editMetaLines(w)...)

	status := fmt.Sprintf("第 %d/%d 个文件 · %d 行", m.editView.cursor+1, len(files), len(lines))
	if m.editView.running {
		status += " · 运行中"
	}
	f.status = dimStyle.Render(status)
	return f.render(body)
}

// editMetaLines 下方元数据：只展示当前渲染文件的文件名与元数据（类型 / 大小 / 修改时间 / 句柄）。
func (m appModel) editMetaLines(width int) []string {
	file := m.editView.currentFile()
	if file == nil {
		return fitLines([]string{"", ""}, 2)
	}
	touched, _, _ := editSnapshot()
	status := touched[file.Path]

	name := file.Path
	switch {
	case status.Failed:
		// 最近一次运行报错 / 警告 → 红色
		name = editFailStyle.Render(name)
	case !status.At.IsZero():
		// 最近被 AI 读取或操作过 → 黄色
		name = editTouchedStyle.Render(name)
	default:
		// 其余历史文件 → 蓝色
		name = editFileStyle.Render(name)
	}
	// 文件名居中显示；可运行文件在同行右端显示紫色「▶ R键运行」提示。
	nameLine := centerBlock([]string{name}, width)[0]
	if isRunnableDoc(file.Path) {
		hint := editRunStyle.Render("▶ R键运行")
		if m.editView.running {
			hint = editRunStyle.Render("▶ 运行中…")
		}
		nameLine = rightAlignedHint(nameLine, hint, width)
	}

	size := file.Size
	modText := "—"
	if full := filepath.Join(documentsRoot(), filepath.FromSlash(file.Path)); full != "" {
		if info, err := os.Stat(full); err == nil {
			size = info.Size()
			modText = info.ModTime().Format("2006-01-02 15:04:05")
		}
	}
	kind := strings.ToLower(filepath.Ext(file.Path))
	if kind == "" {
		kind = "无扩展名"
	}
	handle := status.Handle
	if handle == "" {
		// 句柄其实早已登记在统一句柄库中：直接按路径查询，不重复登记、也不显示「—」。
		rel := filepath.ToSlash(filepath.Join("documents", file.Path))
		if res, ok := resource.ForPath(rel); ok {
			handle = res.Handle
		}
	}
	if handle == "" {
		handle = "—"
	}
	meta := fmt.Sprintf("  类型 %s · 大小 %s · 修改 %s · 句柄 %s",
		kind, humanSize(size), modText, handle)

	return fitLines([]string{fitWidth(nameLine, width), fitWidth(dimStyle.Render(meta), width)}, 2)
}

// ==== 小工具 ====

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// padLeft 左侧补空格到指定显示宽度
func padLeft(s string, width int) string {
	if w := dispWidth(s); w < width {
		return strings.Repeat(" ", width-w) + s
	}
	return s
}

// rightAlignedHint 把提示贴到行尾：左侧内容过长时截断，保证提示完整可见。
func rightAlignedHint(line, hint string, width int) string {
	hintW := dispWidth(hint)
	if hintW >= width {
		return hint
	}
	maxBase := width - hintW - 1
	base := line
	if dispWidth(base) > maxBase {
		base = truncTo(base, maxInt(1, maxBase))
	}
	pad := width - dispWidth(base) - hintW
	if pad < 1 {
		pad = 1
	}
	return base + strings.Repeat(" ", pad) + hint
}
