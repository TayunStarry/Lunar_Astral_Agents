package storyboard

// emit_test.go 推送协议压测：验证增量、分块、限流与序号单调性。
//
// 这些断言对应前端卡死的三个直接原因：
//   1. 单条推送不得携带完整镜头表（否则体量随镜头数线性膨胀）；
//   2. 单条推送的过程文本必须有上限（否则体量随推理长度膨胀）；
//   3. 过程文本分块后不得丢失或重复（否则前端要么缺字、要么重复追加）。

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// newTestTask 构造一个隔离的任务状态，并按调度器的方式登记到任务表与顺序表。
func newTestTask(t *testing.T, taskID string) *taskState {
	t.Helper()
	now := time.Now().Format(time.RFC3339)
	state := &taskState{
		view:  TaskView{TaskID: taskID, Status: statusPlanning, CreatedAt: now, UpdatedAt: now},
		stage: stagePlanning,
	}
	taskMu.Lock()
	tasks[taskID] = state
	taskOrder = append(taskOrder, taskID)
	taskMu.Unlock()
	t.Cleanup(func() {
		taskMu.Lock()
		consumePendingLocked(state)
		delete(tasks, taskID)
		kept := taskOrder[:0]
		for _, id := range taskOrder {
			if id != taskID {
				kept = append(kept, id)
			}
		}
		taskOrder = kept
		taskMu.Unlock()
	})
	return state
}

// buildPush 在锁内生成一次推送。
func buildPush(t *testing.T, state *taskState, full bool) TaskView {
	t.Helper()
	taskMu.Lock()
	defer taskMu.Unlock()
	view, ok := buildPushLocked(state, full)
	if !ok {
		t.Fatalf("期望生成推送，实际为空")
	}
	return view
}

// applyEvent 在锁内合并一次增量事件，并登记待推送。
func applyEvent(t *testing.T, state *taskState, event ProgressEvent) {
	t.Helper()
	taskMu.Lock()
	defer taskMu.Unlock()
	applyEventLocked(state, event)
	schedulePushLocked(state)
}

func TestThinkingPushCarriesOnlyDelta(t *testing.T) {
	state := newTestTask(t, "task-thinking")
	applyEvent(t, state, ProgressEvent{Status: statusPlanning, Event: eventThinking, ThoughtDelta: "推理第一行\n"})

	view := buildPush(t, state, false)
	if view.V != pushVersion {
		t.Fatalf("推送协议版本 = %d，期望 %d", view.V, pushVersion)
	}
	if len(view.Shots) != 0 {
		t.Fatalf("思考推送不应携带完整镜头表，实际 %d 镜", len(view.Shots))
	}
	if len(view.ShotPatch) != 0 {
		t.Fatalf("思考推送不应携带镜头增量，实际 %d 镜", len(view.ShotPatch))
	}
	if view.Delta != "推理第一行\n" {
		t.Fatalf("过程文本增量 = %q", view.Delta)
	}
}

func TestThoughtDeltaIsChunkedWithoutLoss(t *testing.T) {
	state := newTestTask(t, "task-chunk")
	// 构造超过单条上限的中文文本：每个汉字 3 字节，确保按 rune 边界切分。
	var builder strings.Builder
	for i := 0; i < 3000; i++ {
		builder.WriteString("星月")
	}
	origin := builder.String()
	applyEvent(t, state, ProgressEvent{Event: eventThinking, ThoughtDelta: origin})

	var rebuilt strings.Builder
	for round := 0; round < 200; round++ {
		taskMu.Lock()
		view, ok := buildPushLocked(state, false)
		taskMu.Unlock()
		if !ok {
			break
		}
		if len(view.Delta) > maxPushDeltaBytes {
			t.Fatalf("第 %d 条推送过程文本 %d 字节，超过上限 %d", round, len(view.Delta), maxPushDeltaBytes)
		}
		rebuilt.WriteString(view.Delta)
	}
	if rebuilt.String() != origin {
		t.Fatalf("分块后文本不一致：期望 %d 字节，实际 %d 字节", len(origin), rebuilt.Len())
	}
}

func TestShotPatchOnlyCarriesChangedShot(t *testing.T) {
	state := newTestTask(t, "task-shots")
	shots := make([]ShotView, 0, 3)
	for index := 1; index <= 3; index++ {
		shots = append(shots, ShotView{Index: index, Scene: "镜次", Status: shotPending, Prompt: strings.Repeat("画面", 200)})
	}
	applyEvent(t, state, ProgressEvent{Event: eventShotPlanned, Shots: cloneShots(shots)})
	first := buildPush(t, state, false)
	if len(first.ShotPatch) != 3 || len(first.Shots) != 0 {
		t.Fatalf("首个镜头推送应为 3 镜增量，实际增量 %d 全量 %d", len(first.ShotPatch), len(first.Shots))
	}

	// 只改第 2 镜：推送必须只带这一镜。
	next := cloneShots(shots)
	next[1].Status = shotDrawing
	applyEvent(t, state, ProgressEvent{Event: eventShotStarted, ShotIndex: 2, Shots: next})
	second := buildPush(t, state, false)
	if len(second.Shots) != 0 {
		t.Fatalf("状态未整体变化时不应回退全量，实际 %d 镜", len(second.Shots))
	}
	if len(second.ShotPatch) != 1 || second.ShotPatch[0].Index != 2 {
		t.Fatalf("镜头增量应只含第 2 镜，实际 %+v", second.ShotPatch)
	}
	if second.ShotCount != 3 {
		t.Fatalf("镜头总数 = %d，期望 3", second.ShotCount)
	}

	// 删减镜头：整体变化必须回退全量，前端据此清掉多余行。
	applyEvent(t, state, ProgressEvent{Event: eventReviewConfirmed, Shots: cloneShots(next[:2])})
	third := buildPush(t, state, false)
	if len(third.Shots) != 2 {
		t.Fatalf("镜头表缩短时应推送全量 2 镜，实际 %d", len(third.Shots))
	}
}

func TestPushSeqIsMonotonic(t *testing.T) {
	state := newTestTask(t, "task-seq")
	var last uint64
	for index := 0; index < 50; index++ {
		applyEvent(t, state, ProgressEvent{Event: eventThinking, ThoughtDelta: "片段"})
		view := buildPush(t, state, false)
		if view.Seq <= last {
			t.Fatalf("第 %d 次推送序号 %d 未递增（上一条 %d）", index, view.Seq, last)
		}
		last = view.Seq
	}
}

func TestPushPayloadStaysBounded(t *testing.T) {
	state := newTestTask(t, "task-size")
	// 模拟规划阶段：40 镜 + 每镜都带长提示词。
	shots := make([]ShotView, 0, 40)
	for index := 1; index <= 40; index++ {
		shots = append(shots, ShotView{
			Index:     index,
			Act:       "第一幕",
			Scene:     "1-" + strings.Repeat("0", 3),
			Narration: strings.Repeat("叙事", 60),
			Prompt:    strings.Repeat("画面要求", 80),
			Status:    shotPending,
		})
	}
	applyEvent(t, state, ProgressEvent{Event: eventShotPlanned, Shots: shots})
	// 先消费掉镜头首次推送。
	buildPush(t, state, false)

	applyEvent(t, state, ProgressEvent{Event: eventThinking, ThoughtDelta: strings.Repeat("推理", 200)})
	view := buildPush(t, state, false)
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("推送序列化失败: %v", err)
	}
	// 单条增量推送上限 = 过程文本分块上限 + 少量标量字段
	if limit := maxPushDeltaBytes + 1024; len(raw) > limit {
		t.Fatalf("增量推送 %d 字节，超过上限 %d：思考阶段不应携带镜头表", len(raw), limit)
	}
}
