package library

// store.go 编辑状态中心：记录 AI 操作、推断文件名配色、维护最近 10 次操作。
//
// 数据方向是单向的：文件编辑智能体每完成一次操作就上报一次，资源库只读取与广播，
// 不接受任何正文修改。

import (
	"sort"
	"strings"
	"time"
)

// SetBroadcast 注入 WebSocket 结构化广播（由装配层调用；nil 表示不广播）。
func SetBroadcast(fn func(msgType string, data any)) {
	broadcastMu.Lock()
	broadcast = fn
	broadcastMu.Unlock()
}

// PublishEdit 上报一次文件操作（线程安全、幂等）。
func PublishEdit(notice EditNotice) {
	path := normalizeRel(notice.Path)
	if path == "" || rootOf(path) == "" {
		return
	}
	at := notice.At
	if at.IsZero() {
		at = time.Now()
	}
	stateMu.Lock()
	state := states[path]
	if state == nil {
		state = &FileState{Path: path, Status: StatusNormal}
		states[path] = state
	}
	// 运行失败状态只在「运行」操作里更新：一次失败保持红色，直到下次运行成功。
	if notice.Op == "run" {
		state.hasRun = true
		state.runOK = !notice.Failed
		state.Failed = notice.Failed
	}
	if notice.Handle != "" {
		state.Handle = notice.Handle
	}
	state.Op = notice.Op
	state.At = at.Unix()
	if notice.Op != "run" {
		state.Records = append(state.Records, EditRecord{
			Op:        notice.Op,
			Added:     clipRunes(notice.Added, maxSnippetRunes),
			Removed:   clipRunes(notice.Removed, maxSnippetRunes),
			StartLine: notice.StartLine,
			Lines:     notice.Lines,
			At:        at.Unix(),
		})
		state.Records = trimHistory(state.Records)
	}
	state.Status = statusOf(state)
	snapshot := cloneState(state)
	version++
	msgVersion := version
	stateMu.Unlock()

	if broadcast == nil {
		return
	}
	broadcastMu.RLock()
	fn := broadcast
	broadcastMu.RUnlock()
	if fn == nil {
		return
	}
	fn("library_edit", map[string]any{
		"version": msgVersion,
		"file":    snapshot,
	})
}

// Snapshot 返回全部文件状态（按路径升序）。
func Snapshot() EditSnapshot {
	stateMu.RLock()
	defer stateMu.RUnlock()
	files := make([]FileState, 0, len(states))
	for _, state := range states {
		files = append(files, cloneState(state))
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return EditSnapshot{Version: version, Files: files}
}

// Clear 清空状态（数据导出 / 测试用）。
func Clear() {
	stateMu.Lock()
	states = map[string]*FileState{}
	version++
	stateMu.Unlock()
}

// statusOf 推断文件名配色：运行失败红 > 运行成功绿 > 有操作记录黄 > 默认。
func statusOf(state *FileState) string {
	switch {
	case state.Failed:
		return StatusError
	case state.hasRun && state.runOK:
		return StatusOK
	case state.At > 0:
		return StatusActive
	default:
		return StatusNormal
	}
}

// trimHistory 只保留最近 maxEditHistory 次操作。
func trimHistory(records []EditRecord) []EditRecord {
	if len(records) <= maxEditHistory {
		return records
	}
	return append([]EditRecord(nil), records[len(records)-maxEditHistory:]...)
}

// cloneState 复制状态（避免锁外共享可变切片）。
func cloneState(state *FileState) FileState {
	out := *state
	out.Records = append([]EditRecord(nil), state.Records...)
	return out
}

// clipRunes 按字符截断片段（超出时在尾部标注省略）。
func clipRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "…"
}
