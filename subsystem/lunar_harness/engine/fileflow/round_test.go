package fileflow

// round_test.go 统一库的跨轮验收：自由创作全量留存、指派模式只留步骤 / 错误、
// change 窗口合并后仍可回退。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/internal/testenv"
)

// recordCallOp 执行一次带调用轮次的快照 + 增量采集 + 落库。
func recordCallOp(t *testing.T, callID, flowID string, seq int, op, target, pathRel, content string) {
	t.Helper()
	snapshot, err := Capture(flowID, seq, op, target, []string{target})
	if err != nil {
		t.Fatalf("抓取快照失败: %v", err)
	}
	writeTestFile(t, pathRel, content)
	entry := snapshot.Entries[0]
	built, err := BuildOp(OpInput{
		FlowID: flowID, Seq: seq, Step: seq, Mode: ModeCreator, CallID: callID,
		Op: op, Target: target, PathRel: entry.Rel, PathAbs: entry.Abs, Tool: "test_tool",
		Snapshot: snapshot,
	})
	if err != nil {
		t.Fatalf("采集内容增量失败: %v", err)
	}
	if err := built.Commit(Review{OK: true}); err != nil {
		t.Fatalf("写入增量记录失败: %v", err)
	}
}

// countRows 统计表行数（测试内省用）。
func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("统计 %s 失败: %v", table, err)
	}
	return count
}

// TestFreeRoundStoresVisibleContext 自由创作一轮写入后，下一轮能读到全部可见信息。
func TestFreeRoundStoresVisibleContext(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })

	callID, err := BeginCall(ModeCreator)
	if err != nil {
		t.Fatalf("开始轮次失败: %v", err)
	}
	recordCallOp(t, callID, "free-flow", 1, OpCreate, "/document/notes/a.md", "documents/notes/a.md", "初稿\n")
	steps := []RoundStep{{Agent: "writer", Task: "写文章"}}
	if err := FinishRound(callID, ModeCreator, CallStatusDone, "上一轮收尾", steps,
		[]string{"参考资料 A"}, []string{"下一轮继续润色"}, []string{"上次执行失败"}); err != nil {
		t.Fatalf("归档轮次失败: %v", err)
	}

	if _, err := BeginCall(ModeCreator); err != nil {
		t.Fatalf("开始第二轮失败: %v", err)
	}
	prev, err := PrevRound(ModeCreator)
	if err != nil {
		t.Fatalf("读取上一轮失败: %v", err)
	}
	if prev.Call.CallID != callID || prev.Call.Summary != "上一轮收尾" {
		t.Fatalf("上一轮元信息不正确: %+v", prev.Call)
	}
	if len(prev.Steps) != 1 || prev.Steps[0].Task != "写文章" {
		t.Fatalf("上一轮步骤不正确: %+v", prev.Steps)
	}
	if len(prev.References) != 1 || prev.References[0] != "参考资料 A" {
		t.Fatalf("参考资料未保留: %+v", prev.References)
	}
	if len(prev.Memos) != 1 || prev.Memos[0] != "下一轮继续润色" {
		t.Fatalf("备忘录未保留: %+v", prev.Memos)
	}
	if len(prev.Errors) != 1 {
		t.Fatalf("错误未保留: %+v", prev.Errors)
	}
	if len(prev.Changes) == 0 || prev.Changes[0].PathRel != "documents/notes/a.md" {
		t.Fatalf("上一轮变更未保留: %+v", prev.Changes)
	}
}

// TestAssignedRoundPersistsOnlyStepsAndErrors 指派模式不落参考资料与备忘录，但保留步骤、错误与变更。
func TestAssignedRoundPersistsOnlyStepsAndErrors(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })

	callID, err := BeginCall(ModeAssigned)
	if err != nil {
		t.Fatalf("开始轮次失败: %v", err)
	}
	recordCallOp(t, callID, "assign-flow", 1, OpCreate, "/document/notes/a.md", "documents/notes/a.md", "产物\n")
	if err := FinishRound(callID, ModeAssigned, CallStatusDone, "不应保存的收尾",
		[]RoundStep{{Agent: "engineer", Task: "实现功能"}},
		[]string{"不应保存的参考"}, []string{"不应保存的备忘"}, []string{"需要关注的错误"}); err != nil {
		t.Fatalf("归档轮次失败: %v", err)
	}

	db, err := deltaConn()
	if err != nil {
		t.Fatalf("打开统一库失败: %v", err)
	}
	if got := countRows(t, db, `"`+referenceTable+`"`); got != 0 {
		t.Fatalf("指派模式不得写 references，实际 %d 行", got)
	}
	if got := countRows(t, db, memoTable); got != 0 {
		t.Fatalf("指派模式不得写 memos，实际 %d 行", got)
	}
	if got := countRows(t, db, stepAssignTable); got != 1 {
		t.Fatalf("指派步骤应落库，实际 %d 行", got)
	}
	if got := countRows(t, db, errorAssignTable); got != 1 {
		t.Fatalf("指派错误应落库，实际 %d 行", got)
	}

	prev, err := PrevRound(ModeAssigned)
	if err != nil {
		t.Fatalf("读取上一轮失败: %v", err)
	}
	if len(prev.References) != 0 || len(prev.Memos) != 0 {
		t.Fatalf("指派模式上一轮不应带参考 / 备忘: %+v", prev)
	}
	if len(prev.Steps) != 1 || len(prev.Errors) != 1 || len(prev.Changes) != 1 {
		t.Fatalf("指派模式上一轮信息不完整: %+v", prev)
	}

	records, baseline, err := ChangesOfPath("documents/notes/a.md", maxChangeWindow)
	if err != nil {
		t.Fatalf("查询文件变更失败: %v", err)
	}
	if len(records) != 1 || baseline != nil {
		t.Fatalf("应可查询任意文件的变更: records=%d baseline=%+v", len(records), baseline)
	}
}

// TestChangeWindowMergesAndRollbackStillWorks 超过 10 次后合并进原始基准，但仍可只靠数据库回退。
func TestChangeWindowMergesAndRollbackStillWorks(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(func() { _ = CloseDeltaStore() })

	callID, err := BeginCall(ModeCreator)
	if err != nil {
		t.Fatalf("开始轮次失败: %v", err)
	}
	flowID := "window-flow"
	const total = 12
	for seq := 1; seq <= total; seq++ {
		op := OpWrite
		if seq == 1 {
			op = OpCreate
		}
		recordCallOp(t, callID, flowID, seq, op, "/document/notes/w.md", "documents/notes/w.md",
			fmt.Sprintf("version-%d\n", seq))
	}

	records, baseline, err := ChangesOfPath("documents/notes/w.md", maxChangeWindow)
	if err != nil {
		t.Fatalf("查询变更窗口失败: %v", err)
	}
	if len(records) != maxChangeWindow {
		t.Fatalf("可见窗口应保留 %d 次，实际 %d", maxChangeWindow, len(records))
	}
	if baseline == nil || baseline.MergedCount != total-maxChangeWindow {
		t.Fatalf("更早的变更应合并进原始基准: %+v", baseline)
	}
	if baseline.BaseBeforeSize != 0 || baseline.LatestAfterHash == "" {
		t.Fatalf("原始基准应保留合并前的首尾内容信息: %+v", baseline)
	}
	if records[0].Seq != total-maxChangeWindow+1 || records[len(records)-1].Seq != total {
		t.Fatalf("窗口应是最新操作: %+v", records)
	}

	// flow_deltas 仍保留完整历史，合并只裁剪可见窗口
	db, err := deltaConn()
	if err != nil {
		t.Fatalf("打开统一库失败: %v", err)
	}
	if got := countRows(t, db, deltaTable); got < total {
		t.Fatalf("内容增量不应因窗口合并而丢失，实际 %d 行", got)
	}

	// 去掉快照，证明回退仍只靠数据库内容增量
	if err := os.RemoveAll(filepath.Join(config.LocalDir, filepath.FromSlash(SnapshotRootRel), flowID)); err != nil {
		t.Fatalf("删除快照目录失败: %v", err)
	}
	result, err := RollbackTo(flowID, 0)
	if err != nil {
		t.Fatalf("回退失败: %v", err)
	}
	if !result.OK || result.Reverted != total {
		t.Fatalf("合并后仍应能回退全部 %d 步: %+v", total, result)
	}
	if _, statErr := os.Stat(filepath.Join(config.LocalDir, "documents", "notes", "w.md")); !os.IsNotExist(statErr) {
		t.Fatalf("回退后文件应被删除: %v", statErr)
	}
}
