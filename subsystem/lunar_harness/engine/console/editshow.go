package console

// editshow.go「编辑显示」屏：文档库文件清单 + 内容展示（纯展示层）。
//
// 数据方向是单向的：
//   - 文件编辑智能体每完成一次操作，调用 PublishFileEdit 上报（相对 documents 的路径、
//     操作名、本次新增/置换的文本片段与起始行）；
//   - 控制台只读取 document 目录与上报状态做展示，不接受任何手动触发或编辑入口。
//
// 因此「本次运行周期」= 进程生命周期：触碰过的文件一直保持黄色，直到进程退出。

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// ==== 上报（文件编辑智能体 → 控制台） ====

// FileEditNotice 一次文件操作的上报内容。
type FileEditNotice struct {
	// Path 相对 documents 根目录的路径（统一用 / 分隔）
	Path string
	// Op 操作名（read / write / create / replace / move / delete …）
	Op string
	// Snippet 本次新增或置换的文本片段（读取类操作留空）
	Snippet string
	// StartLine 片段在文件中的起始行（1 基；<=0 表示由界面按文本匹配定位）
	StartLine int
	// SnippetLines 片段的行数（<=0 时按 1 行处理）
	SnippetLines int
	// Handle 该文件当前登记的资源句柄（如 @D.019；无句柄时为空串）
	Handle string
	// Failed 上次运行该文件时是否报错 / 警告（仅 Op == "run" 时更新）
	Failed bool
	// At 上报时刻
	At time.Time
}

// FileEditStatus 单个文件最近一次操作的上报状态。
type FileEditStatus struct {
	Op           string
	Snippet      string
	StartLine    int
	SnippetLines int
	Handle       string
	Failed       bool
	At           time.Time
}

var (
	editMu       sync.RWMutex
	editTouched  = map[string]FileEditStatus{}
	editVersion  uint64
	editLastPath string
)

// PublishFileEdit 上报一次文件操作（文件编辑智能体调用；幂等、线程安全）。
func PublishFileEdit(notice FileEditNotice) {
	path := normalizeDocPath(notice.Path)
	if path == "" {
		return
	}
	editMu.Lock()
	editVersion++
	// 运行失败状态只在「运行」操作里更新：一次失败后保持红色，直到下次运行成功。
	failed := false
	if prev, ok := editTouched[path]; ok {
		failed = prev.Failed
	}
	if notice.Op == "run" {
		failed = notice.Failed
	}
	status := FileEditStatus{
		Op:           notice.Op,
		Snippet:      notice.Snippet,
		StartLine:    notice.StartLine,
		SnippetLines: notice.SnippetLines,
		Handle:       notice.Handle,
		Failed:       failed,
		At:           notice.At,
	}
	// 读取类操作不覆盖此前的改动片段：绿色高亮要一直指向「最近一次置换或新增」。
	if strings.TrimSpace(status.Snippet) == "" {
		if prev, ok := editTouched[path]; ok && strings.TrimSpace(prev.Snippet) != "" {
			status.Snippet = prev.Snippet
			status.StartLine = prev.StartLine
			status.SnippetLines = prev.SnippetLines
		}
	}
	editTouched[path] = status
	editLastPath = path
	editMu.Unlock()
	wakeTUI()
}

// ClearFileEdits 清空上报状态（数据导出/测试用；正常运行时不需要）。
func ClearFileEdits() {
	editMu.Lock()
	editTouched = map[string]FileEditStatus{}
	editVersion++
	editLastPath = ""
	editMu.Unlock()
	wakeTUI()
}

// editSnapshot 读取上报状态副本与版本号。
func editSnapshot() (map[string]FileEditStatus, string, uint64) {
	editMu.RLock()
	defer editMu.RUnlock()
	out := make(map[string]FileEditStatus, len(editTouched))
	for path, status := range editTouched {
		out[path] = status
	}
	return out, editLastPath, editVersion
}

// normalizeDocPath 规整相对 documents 的路径（去前缀、统一分隔符）。
func normalizeDocPath(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	path = strings.TrimPrefix(path, "/document/")
	path = strings.TrimPrefix(path, "document/")
	path = strings.TrimPrefix(path, "documents/")
	path = strings.TrimPrefix(path, "/")
	if path == ".." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return ""
	}
	return path
}

// documentsRoot 文档库物理目录（与控制台其它部分一致：LocalDir/documents）。
func documentsRoot() string {
	if abs, err := filepath.Abs(filepath.Join(config.LocalDir, "documents")); err == nil {
		return abs
	}
	return filepath.Join(config.LocalDir, "documents")
}

// ==== 视图状态 ====

// editFile 文件清单条目。
type editFile struct {
	Path string // 相对 documents
	Name string
	Size int64
}

// editState 「编辑显示」屏状态。
type editState struct {
	cursor  int
	offset  int
	lines   int
	files   []editFile
	running bool      // 是否有「运行当前文件」正在进行（防重入；不展示时长）
	loaded  time.Time // 目录扫描时刻（目录 mtime 变化时重扫）
	dirMod  time.Time
	version uint64

	// 当前文件内容缓存
	cachePath string
	cacheMod  time.Time
	cacheVer  uint64
	cacheText []string
}

// newEditState 构建初始状态。
func newEditState() editState { return editState{} }

// reset 丢弃内容缓存（重新进入屏幕时调用；文件清单保留）。
func (s *editState) reset() {
	s.offset = 0
	s.cachePath = ""
	s.cacheText = nil
}

// sync 刷新文件清单与当前文件内容（Update 与 View 共用）。
func (s *editState) sync(width, height int) {
	s.reloadFiles()
	if s.cursor >= len(s.files) {
		s.cursor = maxInt(0, len(s.files)-1)
	}
	s.lines = len(s.visibleLines(width, height))
}

// reloadFiles 扫描 documents 目录（目录 mtime 变化超过阈值或首次时重扫）。
func (s *editState) reloadFiles() {
	root := documentsRoot()
	info, err := os.Stat(root)
	if err != nil {
		if len(s.files) == 0 {
			s.files = nil
		}
		s.loaded = time.Now()
		return
	}
	if !s.loaded.IsZero() && info.ModTime().Equal(s.dirMod) {
		return
	}
	files := make([]editFile, 0, 128)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		size := int64(0)
		if fi, fiErr := d.Info(); fiErr == nil {
			size = fi.Size()
		}
		files = append(files, editFile{
			Path: filepath.ToSlash(rel),
			Name: d.Name(),
			Size: size,
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	s.files = files
	s.dirMod = info.ModTime()
	s.loaded = time.Now()
	s.version++
}

// currentFile 当前选中的文件（无文件时返回 nil）。
func (s *editState) currentFile() *editFile {
	if s.cursor < 0 || s.cursor >= len(s.files) {
		return nil
	}
	return &s.files[s.cursor]
}

// visibleLines 当前文件的内容行（带行号），并按上报的片段做绿色高亮。
func (s *editState) visibleLines(width, _ int) []string {
	f := s.currentFile()
	if f == nil {
		return nil
	}
	text := s.readFile(f.Path)
	touched, _, _ := editSnapshot()
	status, hasStatus := touched[f.Path]
	highlightFrom, highlightTo := 0, 0
	if hasStatus && strings.TrimSpace(status.Snippet) != "" {
		highlightFrom, highlightTo = snippetRange(text, status)
	}
	noW := maxInt(3, len(itoa(len(text))))
	const gutter = 2 // " │ "
	contentW := maxInt(8, width-gutter-noW-1)
	out := make([]string, 0, len(text))
	for i, line := range text {
		no := i + 1
		segments := wrapPlain(line, contentW)
		highlight := no >= highlightFrom && no <= highlightTo
		blankNo := strings.Repeat(" ", noW)
		for j, segment := range segments {
			noText := blankNo
			if j == 0 {
				noText = editLineNoStyle.Render(padLeft(itoa(no), noW))
			}
			body := ""
			switch {
			case highlight:
				body = editHiliteStyle.Render(segment)
			case segment != "":
				body = labelStyle.Render(segment)
			}
			out = append(out, noText+ruleStyle.Render(" │ ")+body)
		}
	}
	return out
}

// readFile 读取文件内容并缓存（按路径 + mtime + 上报版本失效）。
func (s *editState) readFile(rel string) []string {
	full := filepath.Join(documentsRoot(), filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return []string{"（无法读取该文件：" + rel + "）"}
	}
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return []string{"（读取失败：" + readErr.Error() + "）"}
	}
	if strings.TrimSpace(s.cachePath) == rel && s.cacheMod.Equal(info.ModTime()) {
		return s.cacheText
	}
	text := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	// 去掉末尾由换行产生的空行（仅显示上更干净；行号仍以文件实际行为准）
	for len(text) > 0 && text[len(text)-1] == "" {
		text = text[:len(text)-1]
	}
	s.cachePath = rel
	s.cacheMod = info.ModTime()
	s.cacheText = text
	return text
}

// snippetRange 计算高亮行区间：优先用上报的起始行，否则按片段首行文本定位。
func snippetRange(text []string, status FileEditStatus) (int, int) {
	count := status.SnippetLines
	if count <= 0 {
		count = strings.Count(strings.ReplaceAll(status.Snippet, "\r\n", "\n"), "\n") + 1
	}
	if status.StartLine > 0 {
		from := clampInt(status.StartLine, 1, maxInt(1, len(text)))
		return from, minInt(len(text), from+count-1)
	}
	needle := firstNonEmptyLine(status.Snippet)
	if needle == "" {
		return 0, 0
	}
	for i, line := range text {
		if strings.Contains(line, needle) {
			return i + 1, minInt(len(text), i+count)
		}
	}
	return 0, 0
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
