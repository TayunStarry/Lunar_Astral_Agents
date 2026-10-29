package resource

// gc_test.go 验证 GC 表 1 / 表 2 的核心约束

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRootImage 在多媒体根目录写入一张最小 PNG，返回相对路径
func writeRootImage(t *testing.T, root, name string) string {
	t.Helper()
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	rel := "multimedia/" + name
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(abs, png, 0644); err != nil {
		t.Fatalf("写入图片失败: %v", err)
	}
	return rel
}

// TestGC orphanFreeHandleReuse 孤儿句柄进入表 1，并被后续分配优先复用
func TestGC_OrphanFreeHandleReuse(t *testing.T) {
	tmp := setupArchiveTest(t)
	rel := writeRootImage(t, tmp, "orphan.png")
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	res, ok := ForPath(rel)
	if !ok || res.Handle != "@I.001" {
		t.Fatalf("首次登记异常: ok=%v handle=%s", ok, res.Handle)
	}
	// 删除源文件后再次同步：无描述 → 表 1，主表移除
	if err := os.Remove(localAbs(rel)); err != nil {
		t.Fatalf("删除源文件失败: %v", err)
	}
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	if _, ok := ForPath(rel); ok {
		t.Fatal("源丢失且无描述时主表不应保留")
	}
	var orphans int
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM gc_orphan_handle WHERE handle = '@I.001'`).Scan(&orphans)
	if orphans != 1 {
		t.Fatalf("GC 表 1 应记录 @I.001，实际 %d", orphans)
	}
	// 句柄被表 1 移出主表时，GC 表 2 的追踪记录必须同步消失
	var scores int
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM gc_media_score WHERE handle = '@I.001'`).Scan(&scores)
	if scores != 0 {
		t.Fatalf("孤儿句柄的 GC 表 2 记录应同步移除，实际 %d", scores)
	}

	// 新图片优先复用空闲句柄 @I.001
	admitted, err := Admit(AdmitInput{SourceKind: SourceKindBytes, Data: []byte{0x89, 'P', 'N', 'G'}, Name: "new.png"})
	if err != nil {
		t.Fatalf("新资源入库失败: %v", err)
	}
	if admitted.Handle != "@I.001" {
		t.Fatalf("应优先复用空闲句柄 @I.001，实际 %s", admitted.Handle)
	}
}

// TestGC_DescribedHandleNeverRemoved 有文本描述的句柄永不因源丢失被移除
func TestGC_DescribedHandleNeverRemoved(t *testing.T) {
	tmp := setupArchiveTest(t)
	rel := writeRootImage(t, tmp, "described.png")
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	res, ok := ForPath(rel)
	if !ok {
		t.Fatal("登记失败")
	}
	if err := SetSummary(res.ID, "这是一张测试图片", nil); err != nil {
		t.Fatalf("写入描述失败: %v", err)
	}
	if err := os.Remove(localAbs(rel)); err != nil {
		t.Fatalf("删除源文件失败: %v", err)
	}
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	kept, ok := ForPath(rel)
	if !ok || kept.Handle != res.Handle {
		t.Fatalf("有描述的句柄必须保留: ok=%v handle=%s", ok, kept.Handle)
	}
	if kept.State != StateLost {
		t.Fatalf("源丢失后状态应为 lost，实际 %s", kept.State)
	}
}

// TestGC_ScoreAndPurge 积分结算：用过 +1、未用 -1，-20 时删除句柄与物理文件
func TestGC_ScoreAndPurge(t *testing.T) {
	tmp := setupArchiveTest(t)
	rel := writeRootImage(t, tmp, "scored.png")
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	res, ok := ForPath(rel)
	if !ok {
		t.Fatal("登记失败")
	}
	// 文件查询调用：当前调用置 1
	if _, err := Read(res.Handle, 0); err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	var current int
	_ = archiveDB.db.QueryRow(`SELECT current_call FROM gc_media_score WHERE handle = ?`, res.Handle).Scan(&current)
	if current != gcCurrentCallUsed {
		t.Fatalf("文件查询后 current_call 应为 1，实际 %d", current)
	}
	// 启动 GC 结算：用过 → 积分 +1，当前调用归零
	if err := RunStartupGC(); err != nil {
		t.Fatalf("启动 GC 失败: %v", err)
	}
	var points, afterCall int
	_ = archiveDB.db.QueryRow(`SELECT call_points, current_call FROM gc_media_score WHERE handle = ?`, res.Handle).
		Scan(&points, &afterCall)
	if points != 1 || afterCall != 0 {
		t.Fatalf("积分结算异常: points=%d current=%d", points, afterCall)
	}
	// 连续未使用 → 积分 -1
	if err := RunStartupGC(); err != nil {
		t.Fatalf("二次启动 GC 失败: %v", err)
	}
	_ = archiveDB.db.QueryRow(`SELECT call_points FROM gc_media_score WHERE handle = ?`, res.Handle).Scan(&points)
	if points != 0 {
		t.Fatalf("未使用时应 -1，实际 %d", points)
	}

	// 置为 -19 后结算到 -20：删除句柄与物理文件
	if _, err := archiveDB.db.Exec(`UPDATE gc_media_score SET call_points = -19, current_call = 0 WHERE handle = ?`, res.Handle); err != nil {
		t.Fatalf("调整积分失败: %v", err)
	}
	if err := RunStartupGC(); err != nil {
		t.Fatalf("触发清退失败: %v", err)
	}
	if _, ok := Lookup(res.Handle); ok {
		t.Fatal("-20 后句柄记录应被删除")
	}
	if _, err := os.Stat(filepath.Join(tmp, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("物理文件应被删除: %v", err)
	}
	var scoreRows int
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM gc_media_score WHERE handle = ?`, res.Handle).Scan(&scoreRows)
	if scoreRows != 0 {
		t.Fatalf("GC 表 2 记录应删除，实际 %d", scoreRows)
	}
}

// TestGC_DescribedHandleAtMinKeepsHandle 有描述的句柄积分到 -20 时只删文件、保留句柄
func TestGC_DescribedHandleAtMinKeepsHandle(t *testing.T) {
	tmp := setupArchiveTest(t)
	rel := writeRootImage(t, tmp, "kept.png")
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	res, ok := ForPath(rel)
	if !ok {
		t.Fatal("登记失败")
	}
	if err := SetSummary(res.ID, "保留描述", nil); err != nil {
		t.Fatalf("写入描述失败: %v", err)
	}
	if _, err := archiveDB.db.Exec(`UPDATE gc_media_score SET call_points = -19, current_call = 0 WHERE handle = ?`, res.Handle); err != nil {
		t.Fatalf("调整积分失败: %v", err)
	}
	if err := RunStartupGC(); err != nil {
		t.Fatalf("启动 GC 失败: %v", err)
	}
	kept, ok := Lookup(res.Handle)
	if !ok {
		t.Fatal("有文本描述的句柄不得被删除")
	}
	if kept.State != StateLost {
		t.Fatalf("本体清退后状态应为 lost，实际 %s", kept.State)
	}
	if _, err := os.Stat(filepath.Join(tmp, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("物理文件应被删除: %v", err)
	}
	var scores int
	_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM gc_media_score WHERE handle = ?`, res.Handle).Scan(&scores)
	if scores != 0 {
		t.Fatalf("已清退的句柄应退出 GC 表 2，实际 %d", scores)
	}
}

// TestGC_MoveSyncsScoreTable 文件移出/移入追踪根目录时，GC 表 2 实时新增/移除
func TestGC_MoveSyncsScoreTable(t *testing.T) {
	tmp := setupArchiveTest(t)
	rel := writeRootImage(t, tmp, "moved.png")
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	res, ok := ForPath(rel)
	if !ok {
		t.Fatalf("登记失败: %v", ok)
	}
	scoreCount := func() int {
		var count int
		_ = archiveDB.db.QueryRow(`SELECT COUNT(*) FROM gc_media_score WHERE handle = ?`, res.Handle).Scan(&count)
		return count
	}
	if scoreCount() != 1 {
		t.Fatal("根目录图片应进入 GC 表 2")
	}

	// 移出根目录 → 实时退表
	movedRel := "multimedia/generated/moved.png"
	movedAbs := filepath.Join(tmp, filepath.FromSlash(movedRel))
	if err := os.MkdirAll(filepath.Dir(movedAbs), 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.Rename(localAbs(rel), movedAbs); err != nil {
		t.Fatalf("移动文件失败: %v", err)
	}
	RewriteHandlesForMove(rel, movedRel)
	if scoreCount() != 0 {
		t.Fatal("移出追踪根目录后应实时退出 GC 表 2")
	}

	// 移回根目录 → 实时重新建档，积分从 0 开始
	if err := os.Rename(movedAbs, localAbs(rel)); err != nil {
		t.Fatalf("移回文件失败: %v", err)
	}
	RewriteHandlesForMove(movedRel, rel)
	var points, current int
	_ = archiveDB.db.QueryRow(`SELECT call_points, current_call FROM gc_media_score WHERE handle = ?`, res.Handle).
		Scan(&points, &current)
	if points != GCPointsInit || current != 0 {
		t.Fatalf("重新进入追踪范围应以 0/0 建档，实际 points=%d current=%d", points, current)
	}
}

// TestGC_ScopeOnlyRootMedia 表 2 只覆盖 multimedia 根图片/视频与 audios 根哈希音频
func TestGC_ScopeOnlyRootMedia(t *testing.T) {
	if !gcTrackablePath("multimedia/pic.png", KindImage) {
		t.Fatal("multimedia 根图片应可清理")
	}
	if !gcTrackablePath("multimedia/movie.mp4", KindVideo) {
		t.Fatal("multimedia 根视频应可清理")
	}
	if gcTrackablePath("multimedia/generated/pic.png", KindImage) {
		t.Fatal("子目录图片不应进入 GC 表 2")
	}
	if !gcTrackablePath("audios/0123456789abcdef.mp3", KindAudio) {
		t.Fatal("audios 根哈希音频应可清理")
	}
	if gcTrackablePath("audios/voice.mp3", KindAudio) {
		t.Fatal("非哈希命名的音频不应进入 GC 表 2")
	}
	if gcTrackablePath("documents/note.md", KindDocument) {
		t.Fatal("文档不应进入 GC 表 2")
	}
}
