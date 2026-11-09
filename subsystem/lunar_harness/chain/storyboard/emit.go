package storyboard

// emit.go 演绎任务的推送合并、限流与分块。
//
// 为什么需要这一层：规划阶段模型逐行流式返回，每行都会触发一次状态变更。
// 若每条变更都广播一份「标量 + 完整镜头表 + 累计过程文本」的快照，
// 单条消息会随镜头数与文本量线性膨胀，总流量按平方增长，
// 浏览器端每条消息都要重新解析并遍历全部镜头，主线程被彻底占满。
//
// 这里的约定：
//   - 变更先登记到任务状态（标量直接覆盖、镜头按下标去重、过程文本按字节游标续传）；
//   - 推送由定时器合并，同一任务最快 pushInterval 一次；
//   - 单条过程文本增量不超过 maxPushDeltaBytes，超出的部分留到下一轮，绝不丢内容；
//   - 镜头表只在必要时全量（提交 / 审查 / 终态），其余推送只带变化的镜头。

import (
	"sort"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	// pushVersion 当前推送协议版本
	pushVersion = 2
	// pushInterval 同一任务的推送合并窗口：把逐行流式增量压成固定频率
	pushInterval = 120 * time.Millisecond
	// maxPushDeltaBytes 单条推送携带的过程文本字节上限（按 rune 边界切分）
	maxPushDeltaBytes = 2048
)

// schedulePushLocked 登记一次待推送并安排合并推送；调用方必须持有 taskMu。
func schedulePushLocked(state *taskState) {
	state.pushPending = true
	if state.pushTimer != nil {
		return
	}
	state.pushTimer = time.AfterFunc(pushInterval, func() {
		flushPush(state)
	})
}

// flushPush 合并窗口到期：生成并投递一次增量推送。
func flushPush(state *taskState) {
	taskMu.Lock()
	if state.pushTimer == nil {
		// 已被即时推送或状态查询消费，这一轮无需再发。
		taskMu.Unlock()
		return
	}
	state.pushTimer = nil
	view, ok := buildPushLocked(state, false)
	taskMu.Unlock()
	if ok {
		emitTask(view)
	}
}

// buildPushLocked 生成一次推送视图；调用方必须持有 taskMu。
//
// full 为 true 时强制携带完整镜头表（提交 / 审查 / 终态等低频事件）。
// 返回 false 表示没有任何待推送内容。
func buildPushLocked(state *taskState, full bool) (TaskView, bool) {
	if state.pushTimer != nil {
		state.pushTimer.Stop()
		state.pushTimer = nil
	}
	wantFull := full || state.dirtyAll
	if !state.pushPending && !wantFull {
		return TaskView{}, false
	}
	state.pushPending = false
	state.dirtyAll = false

	view := state.view
	view.V = pushVersion
	view.Epoch = taskEpoch
	view.Seq = atomic.AddUint64(&viewSeq, 1)
	state.view.Seq = view.Seq
	view.ShotCount = len(state.view.Shots)
	// 推送不携带累计全文：完整过程文本只在状态查询里返回一次。
	view.Thought = ""
	view.Delta = ""
	view.ThoughtReset = false
	view.Shots = nil
	view.ShotPatch = nil

	if wantFull {
		view.Shots = cloneShots(state.view.Shots)
		state.dirtyShots = nil
	} else if len(state.dirtyShots) > 0 {
		patch := make([]ShotView, 0, len(state.dirtyShots))
		for index := range state.dirtyShots {
			if index >= 0 && index < len(state.view.Shots) {
				patch = append(patch, state.view.Shots[index])
			}
		}
		sort.Slice(patch, func(i, j int) bool { return patch[i].Index < patch[j].Index })
		state.dirtyShots = nil
		view.ShotPatch = patch
	}

	if state.thoughtReset {
		view.ThoughtReset = true
		state.thoughtReset = false
		state.sentThought = 0
	}
	// 过程文本续传：从游标处切一段，超长时分块留到下一轮。
	if len(state.thought) > state.sentThought {
		start := state.sentThought
		end := len(state.thought)
		if end-start > maxPushDeltaBytes {
			// 向前回退到 rune 起点：保证单条推送严格不超过上限，也不会切出半个汉字。
			end = alignRuneEnd(state.thought, start+maxPushDeltaBytes)
			state.pushPending = true
		}
		view.Delta = state.thought[start:end]
		state.sentThought = end
	}
	if state.pushPending && state.pushTimer == nil {
		state.pushTimer = time.AfterFunc(pushInterval, func() {
			flushPush(state)
		})
	}
	return view, true
}

// markChangedShotsLocked 记录相对上一次推送发生变化的镜头；调用方必须持有 taskMu。
//
// 逐镜规划是「在上一次结果末尾追加一镜」，逐镜绘制是「改一个镜头的状态」，
// 两种情况都能只推送差异；只有镜头表整体缩短时才退回全量。
func markChangedShotsLocked(state *taskState, next []ShotView) {
	if state.dirtyAll {
		return
	}
	prev := state.view.Shots
	if len(next) < len(prev) {
		state.dirtyAll = true
		state.dirtyShots = nil
		return
	}
	for index := 0; index < len(prev); index++ {
		if next[index] != prev[index] {
			markShotDirtyLocked(state, index)
		}
	}
	for index := len(prev); index < len(next); index++ {
		markShotDirtyLocked(state, index)
	}
}

// markShotDirtyLocked 登记单个待推送镜头；调用方必须持有 taskMu。
func markShotDirtyLocked(state *taskState, index int) {
	if state.dirtyShots == nil {
		state.dirtyShots = make(map[int]struct{})
	}
	state.dirtyShots[index] = struct{}{}
}

// alignRuneEnd 把字节下标向前回退到合法 UTF-8 起始字节（不切出半个字符）。
func alignRuneEnd(text string, offset int) int {
	if offset > len(text) {
		offset = len(text)
	}
	for offset > 0 && offset < len(text) && !utf8.RuneStart(text[offset]) {
		offset--
	}
	return offset
}
