package memory

// replace_test.go 记忆整理「先写后删 + 校验 + 回滚」的事务语义回归测试。

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// setupReplaceStore 初始化一个独立的记忆库，返回 store 与假嵌入服务。
func setupReplaceStore(t *testing.T) (*Store, *httptest.Server) {
	t.Helper()
	server := startFakeEmbeddingServer(t)
	isolateStorage(t)
	origURL, origModel := config.AgentEmbeddingURL, config.AgentEmbeddingModel
	config.AgentEmbeddingURL = server.URL + "/v1"
	config.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		config.AgentEmbeddingURL, config.AgentEmbeddingModel = origURL, origModel
		server.Close()
	})
	d := newStore(t.TempDir())
	if err := d.EnsureReady(); err != nil {
		t.Fatalf("记忆库初始化失败: %v", err)
	}
	return d, server
}

// TestReplaceTextsKeepsNewAndRemovesOld 正常替换：新内容全部落库，旧条目全部删除。
func TestReplaceTextsKeepsNewAndRemovesOld(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	oldIDs := addTexts(t, d, "今天天气不错", "心情很开心", "写代码很顺利")
	result, err := d.ReplaceTexts(ctx, oldIDs, []string{"整理后天气不错", "整理后心情开心"}, ReplaceOptions{})
	if err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if len(result.CreatedIDs) != 2 || len(result.DeletedIDs) != len(oldIDs) {
		t.Fatalf("替换统计异常: %+v", result)
	}
	assertAbsent(t, d, oldIDs)
	for _, id := range result.CreatedIDs {
		if _, err := d.Get(id); err != nil {
			t.Fatalf("新条目 %s 未落库: %v", id, err)
		}
	}
	if got := d.Count(TypeText); got != 2 {
		t.Fatalf("替换后应剩 2 条文本记忆，实得 %d", got)
	}
}

// TestReplaceTextsRejectsEmptyResult 空整理结果必须拒绝，旧条目一条都不能少。
func TestReplaceTextsRejectsEmptyResult(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	oldIDs := addTexts(t, d, "今天天气不错", "心情很开心")
	if _, err := d.ReplaceTexts(ctx, oldIDs, []string{"   ", ""}, ReplaceOptions{}); err == nil {
		t.Fatal("空结果应返回错误")
	}
	for _, id := range oldIDs {
		if _, err := d.Get(id); err != nil {
			t.Fatalf("空结果不得删除旧条目 %s: %v", id, err)
		}
	}
}

// TestReplaceTextsKeepsOldWhenEmbeddingDown 写入阶段失败时旧条目必须原样保留。
func TestReplaceTextsKeepsOldWhenEmbeddingDown(t *testing.T) {
	d, server := setupReplaceStore(t)
	ctx := context.Background()

	oldIDs := addTexts(t, d, "今天天气不错", "心情很开心")
	server.Close() // 嵌入服务不可用：新条目无法写入

	if _, err := d.ReplaceTexts(ctx, oldIDs, []string{"整理后的天气记录"}, ReplaceOptions{}); err == nil {
		t.Fatal("嵌入不可用时写入应失败")
	}
	for _, id := range oldIDs {
		if _, err := d.Get(id); err != nil {
			t.Fatalf("写入失败不得影响旧条目 %s: %v", id, err)
		}
	}
	if got := d.Count(TypeText); got != len(oldIDs) {
		t.Fatalf("写入失败后文本条目数应保持 %d，实得 %d", len(oldIDs), got)
	}
}

// TestRollbackReplaceRestoresOldAndRemovesNew 事务中途失败的回滚：按快照恢复旧条目、清理本次新建。
// 直接覆盖 rollbackReplace：模拟「新条目已写入、旧条目已删除」后回滚，两者都必须回到原状。
func TestRollbackReplaceRestoresOldAndRemovesNew(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	oldIDs := addTexts(t, d, "今天天气不错", "心情很开心")
	oldSet := make(map[string]struct{}, len(oldIDs))
	for _, id := range oldIDs {
		oldSet[id] = struct{}{}
	}
	snapshots := d.snapshotTextDocs(oldSet)
	if len(snapshots) != len(oldIDs) {
		t.Fatalf("快照条数异常: %d", len(snapshots))
	}
	newID, err := d.addTextNoDedup(ctx, "assistant", "整理后的天气记录", "consolidation")
	if err != nil {
		t.Fatalf("写入新条目失败: %v", err)
	}
	for _, snap := range snapshots {
		if err := d.Delete(ctx, snap.Doc.ID); err != nil {
			t.Fatalf("删除旧条目 %s 失败: %v", snap.Doc.ID, err)
		}
	}
	if err := d.rollbackReplace(ctx, []string{newID}, snapshots); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	for _, id := range oldIDs {
		if _, err := d.Get(id); err != nil {
			t.Fatalf("回滚后旧条目 %s 未恢复: %v", id, err)
		}
	}
	if _, err := d.Get(newID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("回滚后新建条目 %s 应已清理，err=%v", newID, err)
	}
	if got := d.Count(TypeText); got != len(oldIDs) {
		t.Fatalf("回滚后条目数应恢复为 %d，实得 %d", len(oldIDs), got)
	}
}

// TestReplaceTextsIdempotent 同一批替换连续执行两次：不产生副本，旧条目不再存在。
func TestReplaceTextsIdempotent(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	oldIDs := addTexts(t, d, "今天天气不错", "心情很开心")
	texts := []string{"整理后天气不错", "整理后心情开心"}
	first, err := d.ReplaceTexts(ctx, oldIDs, texts, ReplaceOptions{})
	if err != nil {
		t.Fatalf("首次替换失败: %v", err)
	}
	second, err := d.ReplaceTexts(ctx, oldIDs, texts, ReplaceOptions{})
	if err != nil {
		t.Fatalf("二次替换失败: %v", err)
	}
	if len(second.CreatedIDs) != 0 || len(second.ReusedIDs) != len(texts) {
		t.Fatalf("二次替换应全部复用既有条目: %+v", second)
	}
	if got := d.Count(TypeText); got != len(first.CreatedIDs) {
		t.Fatalf("幂等替换后条目数应保持不变，实得 %d", got)
	}
}

// TestReplaceTextsKeepsContentEqualToOld 新条目与旧条目完全同文时，旧 ID 删掉后内容仍必须存在。
func TestReplaceTextsKeepsContentEqualToOld(t *testing.T) {
	d, _ := setupReplaceStore(t)
	ctx := context.Background()

	oldIDs := addTexts(t, d, "今天天气不错", "心情很开心")
	same := "今天天气不错"
	result, err := d.ReplaceTexts(ctx, oldIDs, []string{same}, ReplaceOptions{})
	if err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	for _, id := range oldIDs {
		if _, err := d.Get(id); err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatalf("读取旧条目失败: %v", err)
		}
	}
	newID, ok := d.findExactTextDoc(same, nil)
	if !ok {
		t.Fatal("与旧条目同文的新内容在替换后消失了")
	}
	if len(result.CreatedIDs) != 1 || result.CreatedIDs[0] != newID {
		t.Fatalf("同文内容应新建一份再删旧，实得 %+v", result)
	}
	if got := d.Count(TypeText); got != 1 {
		t.Fatalf("替换后应只剩 1 条文本记忆，实得 %d", got)
	}
}

// addTexts 逐条写入互不相同的文本记忆，返回 ID 列表。
func addTexts(t *testing.T, d *Store, texts ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(texts))
	for _, text := range texts {
		id, err := d.AddText(context.Background(), "user", text)
		if err != nil {
			t.Fatalf("写入测试记忆 %q 失败: %v", text, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// assertAbsent 断言一组 ID 均已不存在。
func assertAbsent(t *testing.T, d *Store, ids []string) {
	t.Helper()
	for _, id := range ids {
		if _, err := d.Get(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("旧条目 %s 应已删除，err=%v", id, err)
		}
	}
}
