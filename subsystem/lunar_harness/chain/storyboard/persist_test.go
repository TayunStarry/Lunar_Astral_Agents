package storyboard

// persist_test.go 校验重启恢复：能还原现场的继续推进，无法还原的标记中断。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/internal/testenv"
)

// resetTasksForTest 清空包级任务表，避免用例之间互相影响。
func resetTasksForTest() {
	taskMu.Lock()
	tasks = make(map[string]*taskState)
	taskOrder = nil
	taskMu.Unlock()
}

// writePersistedTasks 把落盘记录写到隔离目录。
func writePersistedTasks(t *testing.T, dir string, version int, records ...json.RawMessage) {
	t.Helper()
	raw, err := json.Marshal(persistedTasks{Version: version, Tasks: records})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(storyboardTasksFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestRecoverResumesPaintingFromNextShot 绘制阶段重启后从下一个未完成镜头继续。
func TestRecoverResumesPaintingFromNextShot(t *testing.T) {
	dir := testenv.Isolate(t)
	resetTasksForTest()
	t.Cleanup(resetTasksForTest)

	plan := persistedPlan{Title: "雨夜", Shots: []persistedShot{
		{Index: 1, Scene: "门口", Narration: "a", Prompt: "p1"},
		{Index: 2, Scene: "灯下", Narration: "b", Prompt: "p2"},
		{Index: 3, Scene: "街头", Narration: "c", Prompt: "p3"},
	}}
	// 前两镜画面与配音都已完成，第三镜尚未合成。
	shots := []ShotView{
		{Index: 1, Scene: "门口", Status: shotPainted, Handle: "@I.001", VoiceStatus: voiceReady, VoiceDurationMs: 2400},
		{Index: 2, Scene: "灯下", Status: shotPainted, Handle: "@I.002", VoiceStatus: voiceReady, VoiceDurationMs: 2600},
		{Index: 3, Scene: "街头", Status: shotPlanned},
	}
	record := persistedTask{
		View: TaskView{
			TaskID:    "sb-1",
			Status:    statusPainting,
			Stage:     "正在绘制第 3 / 3 镜",
			Progress:  50,
			CreatedAt: time.Now().Format(time.RFC3339),
		},
		Request:      SubmitRequest{Text: "故事"},
		Stage:        stagePainting,
		Story:        "故事",
		Plan:         plan,
		Shots:        shots,
		NextShot:     1, // 故意给错：恢复时应按已绘制镜头修正为 2
		OutputFolder: "雨夜",
	}
	writePersistedTasks(t, dir, 2, mustRaw(t, record))

	RecoverInterrupted()

	taskMu.Lock()
	state, ok := tasks["sb-1"]
	taskMu.Unlock()
	if !ok {
		t.Fatal("任务应被恢复")
	}
	if state.stage != stagePainting {
		t.Fatalf("应回到绘制阶段，实际 %s", state.stage)
	}
	if state.nextShot != 2 {
		t.Fatalf("应从第 3 镜继续（下标 2），实际 %d", state.nextShot)
	}
	if state.view.Status != statusPainting {
		t.Fatalf("对外状态应为 painting，实际 %s", state.view.Status)
	}
	if state.view.Event != eventResumed {
		t.Fatalf("恢复推送事件应为 resumed，实际 %s", state.view.Event)
	}
	if !HasWork() {
		t.Fatal("恢复后的任务应可继续推进")
	}
}

// TestRecoverKeepsCompletedTask 已完成任务原样恢复，不重新执行。
func TestRecoverKeepsCompletedTask(t *testing.T) {
	dir := testenv.Isolate(t)
	resetTasksForTest()
	t.Cleanup(resetTasksForTest)

	record := persistedTask{
		View:  TaskView{TaskID: "sb-done", Status: statusCompleted, Progress: 100},
		Stage: stageDone,
	}
	writePersistedTasks(t, dir, 2, mustRaw(t, record))

	RecoverInterrupted()

	taskMu.Lock()
	state, ok := tasks["sb-done"]
	taskMu.Unlock()
	if !ok || state.stage != stageDone || state.view.Status != statusCompleted {
		t.Fatalf("已完成任务应原样恢复，实际 %+v", state)
	}
	if HasWork() {
		t.Fatal("已完成任务不应参与调度")
	}
}

// TestRecoverMarksLegacyTaskInterrupted 旧格式只有视图，无法续跑，按中断处理。
func TestRecoverMarksLegacyTaskInterrupted(t *testing.T) {
	dir := testenv.Isolate(t)
	resetTasksForTest()
	t.Cleanup(resetTasksForTest)

	legacy := TaskView{TaskID: "sb-legacy", Status: statusPainting, Progress: 40}
	writePersistedTasks(t, dir, 1, mustRaw(t, legacy))

	RecoverInterrupted()

	taskMu.Lock()
	state, ok := tasks["sb-legacy"]
	taskMu.Unlock()
	if !ok {
		t.Fatal("旧任务也应记录为中断")
	}
	if state.view.Status != statusFailed || state.view.Error != recoverReason {
		t.Fatalf("旧任务应标记中断，实际 %+v", state.view)
	}
}

// TestRecoverKeepsPausedTask 暂停任务恢复后仍保持暂停，不自动推进。
func TestRecoverKeepsPausedTask(t *testing.T) {
	dir := testenv.Isolate(t)
	resetTasksForTest()
	t.Cleanup(resetTasksForTest)

	record := persistedTask{
		View:    TaskView{TaskID: "sb-paused", Status: statusPaused},
		Stage:   stageResearching,
		Story:   "故事",
		Paused:  true,
		Request: SubmitRequest{Text: "故事"},
	}
	writePersistedTasks(t, dir, 2, mustRaw(t, record))

	RecoverInterrupted()

	taskMu.Lock()
	state, ok := tasks["sb-paused"]
	taskMu.Unlock()
	if !ok || !state.paused || state.view.Status != statusPaused {
		t.Fatalf("暂停任务应保持暂停，实际 %+v", state)
	}
	if HasWork() {
		t.Fatal("暂停任务不应自动推进")
	}
}
