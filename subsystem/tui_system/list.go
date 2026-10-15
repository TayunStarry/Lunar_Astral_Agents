package TuiSystem

import (
	"fmt"
	"strings"
)

// scrollInfo 列表窗口的位置信息：驱动右侧滚动条与底栏「第几项 / 共几项」。
type scrollInfo struct {
	Items int // 条目总数
	First int // 窗口内首个（部分）可见条目
	Last  int // 窗口内最后一个（部分）可见条目
	Total int // 总行数
	Top   int // 窗口首行行号
	View  int // 可见行数
	Cur   int // 当前选中条目
}

// positionText 底栏位置文本：选中序号 / 总数
func (info scrollInfo) positionText() string {
	if info.Items == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", info.Cur+1, info.Items)
}

// listGutter 列表行构造成本应让出的常驻列数。
//
// 定高窗口在超屏时会把最右侧 2 列用于「间隔 + 滚动条」。为了让行宽既不超窗
// 又在有无滚动条时都保持一致（不因出现滚动条而抖动），列表行统一按
// 「窗口宽度 - listGutter」构造：超屏时恰好填满内容列，未超屏时右侧留白。
const listGutter = 2

// listPane 定高滚动窗口的滚动状态。
//
// 只保存「窗口首行」这一个量：选中项由各屏自身的下标字段持有（保持既有语义
// 与测试契约），滚动偏移则在每次渲染时由选中项反推——单一数据源，
// 既不会出现「光标与滚动错位」，也避免了同一状态存两份而日久失同步。
//
// 列表项按「块」（多行）为单位渲染，滚动以行为粒度，
// 因此「标题 + 副标题 + 描述」这类多行条目会整体滚入滚出，不会被拦腰截断。
type listPane struct {
	offset int
}

// layoutView 计算窗口并渲染为恰好 height 行、width 列的内容：
// 保证选中项完整可见（块高于窗口时对齐块顶，优先展示标题），
// 列表超屏时最右侧占用 1 列绘制滚动条。
func (p *listPane) layoutView(blocks [][]string, cursor, width, height int) ([]string, scrollInfo) {
	info := scrollInfo{Items: len(blocks), Cur: cursor, View: height}
	if width <= 0 || height <= 0 || len(blocks) == 0 {
		return blankLines(height, width), info
	}

	// 行起点与总行数
	starts := make([]int, len(blocks))
	total := 0
	for i, b := range blocks {
		starts[i] = total
		total += maxInt(1, len(b))
	}
	info.Total = total
	cursor = clampInt(cursor, 0, len(blocks)-1)
	info.Cur = cursor

	// 由选中项反推窗口首行，使选中块完整可见
	top := clampInt(p.offset, 0, maxInt(0, total-height))
	bs := starts[cursor]
	be := bs + maxInt(1, len(blocks[cursor])) - 1
	switch {
	case bs < top:
		top = bs
	case be > top+height-1:
		if be-bs+1 >= height { // 块本身高于窗口：对齐块顶，保留标题
			top = bs
		} else {
			top = be - height + 1
		}
	}
	top = clampInt(top, 0, maxInt(0, total-height))
	p.offset = top
	info.Top = top

	// 展开行并切出窗口
	lines := make([]string, 0, total)
	for _, b := range blocks {
		if len(b) == 0 {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, b...)
	}

	// 可见条目区间
	found := false
	for i := range blocks {
		s := starts[i]
		e := s + maxInt(1, len(blocks[i])) - 1
		if e < top {
			continue
		}
		if s > top+height-1 {
			break
		}
		if !found {
			info.First = i
			found = true
		}
		info.Last = i
	}

	// 滚动条（仅超屏时占用最右侧 1 列，其前留 1 列间隔）
	bar := total > height
	contentW := width
	barW := 0
	if bar {
		barW = 2
		contentW = width - barW
	}
	if contentW < 1 {
		contentW, barW = width, 0
		bar = false
	}

	thumbTop, thumbLen := 0, 0
	if bar {
		thumbLen = maxInt(1, height*height/total)
		thumbTop = top * height / total
		if thumbTop+thumbLen > height {
			thumbTop = height - thumbLen
		}
	}

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		var line string
		if j := top + i; j < len(lines) && j < total {
			line = fitWidth(lines[j], contentW)
		} else {
			line = strings.Repeat(" ", contentW)
		}
		if bar {
			glyph, style := "│", faintStyle
			if i >= thumbTop && i < thumbTop+thumbLen {
				glyph, style = "█", valueStyle
			}
			line += " " + style.Render(glyph)
		}
		out = append(out, line)
	}
	return out, info
}

// blankLines 返回 h 行、宽 w 的空行
func blankLines(h, w int) []string {
	if h <= 0 {
		return nil
	}
	blank := strings.Repeat(" ", maxInt(0, w))
	out := make([]string, h)
	for i := range out {
		out[i] = blank
	}
	return out
}

// windowLines 定高行窗口：follow 为真时贴底（跟随最新内容），否则维持 offset。
// 返回窗口内容、收敛后的 offset 与位置信息。日志屏与流式屏共用此实现，
// 保证「跟随 / 暂停回看 / 位置指示」三种行为在全应用内一致。
func windowLines(lines []string, offset, height int, follow bool) ([]string, int, scrollInfo) {
	total := len(lines)
	if height <= 0 {
		return nil, 0, scrollInfo{Total: total}
	}
	maxOff := maxInt(0, total-height)
	if follow {
		offset = maxOff
	}
	offset = clampInt(offset, 0, maxOff)
	end := minInt(offset+height, total)
	info := scrollInfo{Items: total, Total: total, Top: offset, View: height, Cur: offset, First: offset}
	return fitLines(lines[offset:end], height), offset, info
}

// scrollKey 行窗口滚动键：返回新的 offset、follow 与是否消费该键。
// 跟随态视作停在底部（offset=maxOff），因此从「跟随」按上翻页是从底部起算，
// 而不是跳到顶部；滚到底部自动恢复跟随，向上滚动即暂停跟随（便于回看）。
func scrollKey(offset int, key string, total, height int, follow bool) (int, bool, bool) {
	maxOff := maxInt(0, total-height)
	if follow {
		offset = maxOff
	}
	step := maxInt(1, height-1)
	switch key {
	case "up", "k":
		return clampInt(offset-1, 0, maxOff), false, true
	case "pgup":
		return clampInt(offset-step, 0, maxOff), false, true
	case "home", "g":
		return 0, false, true
	case "down", "j":
		next := clampInt(offset+1, 0, maxOff)
		return next, next >= maxOff, true
	case "pgdown":
		next := clampInt(offset+step, 0, maxOff)
		return next, next >= maxOff, true
	case "end", "G":
		return maxOff, true, true
	case "f":
		return offset, !follow, true
	}
	return offset, follow, false
}

// listMove 列表移动通用实现：wrap 为真时首尾循环，否则止于两端
func listMove(cur, delta, n int, wrap bool) int {
	if n <= 0 {
		return 0
	}
	i := cur + delta
	if wrap {
		return ((i % n) + n) % n
	}
	return clampInt(i, 0, n-1)
}

// navAlias 方向键别名归一：列表态把 ← / → / Tab 统一视作 ↑ / ↓
// （尊重不同用户的肌肉记忆，同时让各屏键位保持一致）
func navAlias(key string) string {
	switch key {
	case "left":
		return "up"
	case "right", "tab":
		return "down"
	}
	return key
}

// listNavKey 列表通用导航键：↑/↓（k/j）单步、PgUp/PgDn 翻页、Home/End（g/G）首尾。
// 返回新的下标与是否消费该键：单步遵循 wrap（首尾循环），翻页与首尾键不循环。
func listNavKey(cur int, key string, n, page int, wrap bool) (int, bool) {
	switch key {
	case "up", "k":
		return listMove(cur, -1, n, wrap), true
	case "down", "j":
		return listMove(cur, 1, n, wrap), true
	case "pgup":
		return listMove(cur, -maxInt(1, page), n, false), true
	case "pgdown":
		return listMove(cur, maxInt(1, page), n, false), true
	case "home", "g":
		return listMove(0, 0, n, false), true
	case "end", "G":
		return listMove(n-1, 0, n, false), true
	}
	return cur, false
}

// textPane 可滚动文本窗格（详情、说明等长文本区域）。
// 折行结果按（原文, 宽度）缓存，避免每帧重复折行。
type textPane struct {
	offset int
	src    string
	width  int
	lines  []string
}

// wrap 折行并缓存
func (p *textPane) wrap(raw string, width int) []string {
	if width == p.width && raw == p.src && p.lines != nil {
		return p.lines
	}
	p.src, p.width = raw, width
	p.lines = wrapPlain(raw, width)
	return p.lines
}

// win 切出恰好 height 行的窗口（offset 自动收敛），返回窗口内容与位置信息
func (p *textPane) win(raw string, width, height int) ([]string, scrollInfo) {
	if height <= 0 {
		return nil, scrollInfo{}
	}
	if width <= 0 {
		width = 1
	}
	lines := p.wrap(raw, width)
	p.offset = clampInt(p.offset, 0, maxInt(0, len(lines)-height))
	info := scrollInfo{Items: len(lines), Total: len(lines), Top: p.offset, View: height, Cur: p.offset, First: p.offset}
	top := p.offset
	end := minInt(top+height, len(lines))
	return fitLines(lines[top:end], height), info
}

// key 处理详情类窗格的滚动键；返回是否消费该键
func (p *textPane) key(k string, height int) bool {
	step := maxInt(1, height-1)
	switch k {
	case "up", "k":
		p.offset -= 1
	case "down", "j":
		p.offset += 1
	case "pgup":
		p.offset -= step
	case "pgdown":
		p.offset += step
	case "home", "g":
		p.offset = 0
	case "end", "G":
		p.offset = 1 << 30 // 由 win 收敛到底部
	default:
		return false
	}
	if p.offset < 0 {
		p.offset = 0
	}
	return true
}
