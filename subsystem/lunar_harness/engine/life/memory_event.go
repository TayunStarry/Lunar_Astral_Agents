package life

// memory_event.go 整理记忆事件：全量文本记忆分批整理，带批次意图日志。
//
// 顺序必须是「先删旧、再写新」：记忆库对 0.98 相似内容会判重，
// 若先写新，新内容会被旧条目判定为重复而不落库，随后旧条目被删就会丢数据。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

type memoryEvent struct{ params eventParams }

func newMemoryEvent() Event { return &memoryEvent{params: paramsOf("memory")} }

func (e *memoryEvent) ID() string       { return "memory" }
func (e *memoryEvent) Weight() int      { return e.params.Weight }
func (e *memoryEvent) DailyQuota() int  { return e.params.Quota }
func (e *memoryEvent) NewRun() EventRun { return &memoryRun{} }

type memoryRun struct {
	data memoryRunData
	plan *memoryPlan
}

// selectMemoryCandidates 按触发条件选出本次要整理的条目。
//
// 三条硬规则：可整理条目 ≥ memoryTriggerMin 才触发；只取入库时间最久的
// memoryBatchItems 条；这些条目的入库时长必须都 ≥ memoryMinAge。
// 任一条件不满足都返回空列表与原因，调用方据此直接按「本次整理完成」收尾。
func selectMemoryCandidates(items []memoryItem, now time.Time) ([]memoryItem, string) {
	if len(items) < memoryTriggerMin {
		return nil, fmt.Sprintf("可整理记忆 %d 条，未达到 %d 条触发线", len(items), memoryTriggerMin)
	}
	eligible := make([]memoryItem, 0, len(items))
	for _, item := range items {
		// 缺失时间戳的旧数据按最久远处理，避免被永久排除在整理之外
		if item.Timestamp <= 0 || now.Sub(time.Unix(item.Timestamp, 0)) >= memoryMinAge {
			eligible = append(eligible, item)
		}
	}
	if len(eligible) < memoryBatchItems {
		return nil, fmt.Sprintf("入库满 %d 天的记忆仅 %d 条，不足单次整理的 %d 条",
			memoryMinAgeDays, len(eligible), memoryBatchItems)
	}
	// listAllTextMemories 已按入库时间升序排列，前 memoryBatchItems 条即最久的一批
	return eligible[:memoryBatchItems], ""
}

func (r *memoryRun) Start(ctx context.Context, _ Env) error {
	if err := ensureMemoryReady(); err != nil {
		return err
	}
	if err := recoverJournals(ctx); err != nil {
		return fmt.Errorf("恢复记忆整理日志失败: %w", err)
	}
	items := listAllTextMemories()
	candidates, skipReason := selectMemoryCandidates(items, time.Now())
	if len(candidates) == 0 {
		// 触发条件不满足：不写计划、不调模型，直接按「本次整理完成」收尾
		r.data = memoryRunData{PlanFile: memoryPlanFile, Reason: skipReason}
		logger.Info(Module, "[记忆整理] 本次不整理：%s（可整理共 %d 条）", skipReason, len(items))
		return nil
	}
	batches := buildMemoryBatches(candidates)
	r.data = memoryRunData{PlanFile: memoryPlanFile, Total: len(batches)}
	plan := memoryPlan{ID: fmt.Sprintf("mem-%d", time.Now().UnixNano()), Batches: batches}
	if err := saveMemoryPlan(plan); err != nil {
		return err
	}
	r.plan = &plan
	logger.Info(Module, "[记忆整理] 可整理 %d 条，本次取入库最久的 %d 条，分 %d 批处理",
		len(items), len(candidates), len(batches))
	return nil
}

func (r *memoryRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	if r.data.Total == 0 {
		detail := r.data.Reason
		if detail == "" {
			detail = "没有可整理的记忆"
		}
		return EventStep{Done: true, Record: "小憩了片刻", Detail: detail}, nil
	}
	if r.plan == nil {
		plan, err := loadMemoryPlan()
		if err != nil {
			// 计划文件在最后一批评分后被清理、但状态尚未落盘：视为已完成，避免误报失败。
			return EventStep{Done: true, Record: "小憩了片刻", Detail: "整理计划已不存在，视为完成"}, nil
		}
		r.plan = plan
	}
	if r.data.Index >= len(r.plan.Batches) {
		removeMemoryPlan()
		return EventStep{Done: true, Record: "小憩了片刻", Detail: "全部批次完成"}, nil
	}
	// 崩溃恢复：当前批可能已经写了意图日志（模型已调用、旧条目已删）。
	// 先把它完成，避免重复调用模型或重复删除。
	recovered, err := recoverBatch(ctx, r.plan.ID, r.data.Index)
	if err != nil {
		return EventStep{}, err
	}
	if recovered {
		r.data.Index++
		if r.data.Index >= len(r.plan.Batches) {
			removeMemoryPlan()
			return EventStep{Done: true, Record: "小憩了片刻", Detail: "全部批次完成"}, nil
		}
		return EventStep{Done: false, Detail: fmt.Sprintf("恢复完成第 %d 批", r.data.Index)}, nil
	}
	batch := r.plan.Batches[r.data.Index]
	if err := processMemoryBatch(ctx, r.plan.ID, r.data.Index, batch); err != nil {
		return EventStep{}, err
	}
	r.data.Index++
	if r.data.Index >= len(r.plan.Batches) {
		removeMemoryPlan()
		return EventStep{Done: true, Record: "小憩了片刻", Detail: "全部批次完成"}, nil
	}
	return EventStep{
		Done:   false,
		Detail: fmt.Sprintf("第 %d/%d 批完成，%d 条旧记忆", r.data.Index, len(r.plan.Batches), len(batch.Items)),
	}, nil
}

func (r *memoryRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *memoryRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}

// ==== 计划文件 ====

func saveMemoryPlan(plan memoryPlan) error {
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化记忆整理计划失败: %w", err)
	}
	if err := persistence.WriteFileAtomic(memoryPlanFile, raw); err != nil {
		return fmt.Errorf("写入记忆整理计划失败: %w", err)
	}
	return nil
}

func loadMemoryPlan() (*memoryPlan, error) {
	raw, ok := persistence.ReadFileBytes(memoryPlanFile)
	if !ok {
		return nil, fmt.Errorf("记忆整理计划不存在")
	}
	var plan memoryPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("解析记忆整理计划失败: %w", err)
	}
	return &plan, nil
}

func removeMemoryPlan() {
	path := filepath.Join(config.LocalDir, filepath.FromSlash(memoryPlanFile))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Warn(Module, "[记忆整理] 删除计划文件失败: %v", err)
	}
}

// ==== 单批处理 ====

func processMemoryBatch(ctx context.Context, planID string, index int, batch memoryBatch) error {
	newTexts, err := consolidateBatch(ctx, batch)
	if err != nil {
		return err
	}
	batchID := fmt.Sprintf("%s-%03d", planID, index)
	journal := memoryJournal{
		BatchID:    batchID,
		Status:     "prepared",
		OldEntries: batch.Items,
		NewTexts:   newTexts,
		UpdatedAt:  time.Now().Unix(),
	}
	if err := saveJournal(journal); err != nil {
		return err
	}
	if err := deleteOldEntries(ctx, &journal); err != nil {
		return err
	}
	if err := writeNewTexts(ctx, &journal); err != nil {
		return err
	}
	journal.Status = "done"
	journal.UpdatedAt = time.Now().Unix()
	_ = saveJournal(journal)
	deleteJournal(batchID)
	logger.Info(Module, "[记忆整理] 批次 %s 完成：旧 %d 条 → 新 %d 条", batchID, len(batch.Items), len(newTexts))
	return nil
}

func deleteOldEntries(ctx context.Context, journal *memoryJournal) error {
	for _, item := range journal.OldEntries {
		if idIn(item.ID, journal.DeletedIDs) {
			continue
		}
		if err := memory.Delete(ctx, item.ID); err != nil && !errors.Is(err, memory.ErrNotFound) {
			journal.UpdatedAt = time.Now().Unix()
			_ = saveJournal(*journal)
			return fmt.Errorf("删除旧记忆 %s 失败: %w", item.ID, err)
		}
		journal.DeletedIDs = append(journal.DeletedIDs, item.ID)
		journal.UpdatedAt = time.Now().Unix()
		if err := saveJournal(*journal); err != nil {
			return err
		}
	}
	journal.Status = "deleted"
	journal.UpdatedAt = time.Now().Unix()
	return saveJournal(*journal)
}

func writeNewTexts(ctx context.Context, journal *memoryJournal) error {
	for _, text := range journal.NewTexts {
		id, err := memory.AddText(ctx, "assistant", text)
		if err != nil {
			journal.Status = "write_failed"
			journal.UpdatedAt = time.Now().Unix()
			_ = saveJournal(*journal)
			return fmt.Errorf("写入整理后记忆失败: %w", err)
		}
		journal.WrittenIDs = append(journal.WrittenIDs, id)
		journal.UpdatedAt = time.Now().Unix()
		if err := saveJournal(*journal); err != nil {
			return err
		}
	}
	journal.Status = "written"
	journal.UpdatedAt = time.Now().Unix()
	return saveJournal(*journal)
}

func idIn(id string, list []string) bool {
	for _, value := range list {
		if value == id {
			return true
		}
	}
	return false
}

// ==== 意图日志 ====

func journalPath(batchID string) string {
	return filepath.ToSlash(filepath.Join(memoryJournalDir, batchID+".json"))
}

func saveJournal(journal memoryJournal) error {
	raw, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化记忆整理日志失败: %w", err)
	}
	if err := persistence.WriteFileAtomic(journalPath(journal.BatchID), raw); err != nil {
		return fmt.Errorf("写入记忆整理日志失败: %w", err)
	}
	return nil
}

func loadJournal(path string) (*memoryJournal, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var journal memoryJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return nil, err
	}
	return &journal, nil
}

func deleteJournal(batchID string) {
	path := filepath.Join(config.LocalDir, filepath.FromSlash(journalPath(batchID)))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Warn(Module, "[记忆整理] 删除批次日志 %s 失败: %v", batchID, err)
	}
}

// recoverBatch 恢复当前批次；返回 true 表示确实处理了一个残留日志。
func recoverBatch(ctx context.Context, planID string, index int) (bool, error) {
	batchID := fmt.Sprintf("%s-%03d", planID, index)
	path := filepath.Join(config.LocalDir, filepath.FromSlash(journalPath(batchID)))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	journal, err := loadJournal(path)
	if err != nil {
		return false, err
	}
	if journal.Status == "done" {
		deleteJournal(batchID)
		return true, nil
	}
	logger.Info(Module, "[记忆整理] 恢复批次 %s（状态=%s）", journal.BatchID, journal.Status)
	if err := deleteOldEntries(ctx, journal); err != nil {
		return false, err
	}
	if err := writeNewTexts(ctx, journal); err != nil {
		return false, err
	}
	deleteJournal(batchID)
	return true, nil
}

// recoverJournals 启动/新事件开始时恢复未完成的批次。
func recoverJournals(ctx context.Context) error {
	dir := filepath.Join(config.LocalDir, filepath.FromSlash(memoryJournalDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		journal, err := loadJournal(path)
		if err != nil {
			logger.Warn(Module, "[记忆整理] 读取批次日志 %s 失败，已跳过: %v", entry.Name(), err)
			continue
		}
		if journal.Status == "done" {
			deleteJournal(journal.BatchID)
			continue
		}
		logger.Info(Module, "[记忆整理] 恢复批次 %s（状态=%s）", journal.BatchID, journal.Status)
		if err := deleteOldEntries(ctx, journal); err != nil {
			logger.Warn(Module, "[记忆整理] 恢复批次 %s 删除旧记忆失败: %v", journal.BatchID, err)
			continue
		}
		if err := writeNewTexts(ctx, journal); err != nil {
			logger.Warn(Module, "[记忆整理] 恢复批次 %s 写入新记忆失败: %v", journal.BatchID, err)
			continue
		}
		deleteJournal(journal.BatchID)
	}
	return nil
}

// ==== 记忆库初始化 ====

var (
	memoryMu    sync.Mutex
	memoryReady bool
)

func ensureMemoryReady() error {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	if memoryReady {
		return nil
	}
	memory.Init(config.MemoryDBDir)
	if err := memory.EnsureReady(); err != nil {
		return fmt.Errorf("记忆库初始化失败: %w", err)
	}
	memoryReady = true
	return nil
}
