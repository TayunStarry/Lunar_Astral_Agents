package life

// memory_event.go 整理记忆事件：唯一的整理调度入口。
//
// 每轮只做一件事：取当前全部待合并组（Pending + 伙伴），逐组处理。
// 流程：pending → partner 连边 + 嵌入相似度聚类分组（只留含 Pending 且 ≥2 条的组）
// → 推理模型合并改写（输出一条或多条事实条目）→ 嵌入双向校验（新结果不跑偏、
// 旧事实不丢失）→ 归档旧条目 → ReplaceTexts 事务替换。
//
// 没有任何待合并组时整轮不动作、不消耗额度；组内条目不会因为「够旧/够多」被挑选，
// 也不存在基于条数或时间的强制淘汰兜底。
//
// 落库顺序必须是「先写新、再删旧」：任何一步失败都能保留原条目；
// 引擎层 ReplaceTexts 负责写后校验（数量 + 内容指纹）与失败回滚，
// 因此这里只保留计划/日志的编排，不做逐条删除与写入。
//
// 自动触发默认开启（config.MemoryAutoConsolidate=true，权重 20、额度 12 次/天）；
// 关闭后权重归零，随机挑选跳过本事件，仍可手动/按需触发：
//   - 启动参数 -memory-consolidate-once；
//   - life.RequestMemoryConsolidation("...") 或 life.RequestEvent("memory", "...")。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

type memoryEvent struct{ params eventParams }

func newMemoryEvent() Event { return &memoryEvent{params: paramsOf("memory")} }

func (e *memoryEvent) ID() string { return "memory" }

// Weight 自动触发权重：自动整理开启且存在待合并条目时才参与随机挑选。
// 没有待合并组时返回 0——整理无事可做，不应空跑一次并占用当日额度；
// 手动触发走 RequestEvent，不经过权重。
func (e *memoryEvent) Weight() int {
	if !config.MemoryAutoConsolidate {
		return 0
	}
	if pendingCountHook() <= 0 {
		return 0
	}
	return e.params.Weight
}
func (e *memoryEvent) DailyQuota() int  { return e.params.Quota }
func (e *memoryEvent) NewRun() EventRun { return &memoryRun{} }

type memoryRun struct {
	data memoryRunData
	plan *memoryPlan
}

// NoWork 本次整理没有产生任何动作（没有待合并组）：调度层据此回滚当日额度。
func (r *memoryRun) NoWork() bool { return r.data.Total == 0 }

// 手动触发（不受自动整理开关影响）。
const defaultMemoryInstruction = "手动触发一次记忆整理"

// RequestMemoryConsolidation 手动登记一次记忆整理，下一空闲 Tick 执行。
// 即使 config.MemoryAutoConsolidate 关闭，本入口仍然可用（按需整理）。
func RequestMemoryConsolidation(instruction string) bool {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		instruction = defaultMemoryInstruction
	}
	return RequestEvent("memory", instruction)
}

// registerStartupConsolidationLocked 处理启动参数 -memory-consolidate-once：
// 登记一次手动整理；调用方需持有锁。
func registerStartupConsolidationLocked() {
	if !config.MemoryConsolidateOnce {
		return
	}
	if strings.TrimSpace(state.PendingEvent) != "" {
		logger.Warn(Module, "[记忆整理] 已存在待执行事件 %s，跳过启动一次性整理", state.PendingEvent)
		return
	}
	state.PendingEvent = "memory"
	state.PendingInstruction = defaultMemoryInstruction
	saveStateLocked()
	logger.Info(Module, "[记忆整理] 已登记启动后一次性整理（-memory-consolidate-once）")
}

func (r *memoryRun) Start(ctx context.Context, _ Env) error {
	if err := ensureMemoryReady(); err != nil {
		return err
	}
	if err := recoverJournals(ctx); err != nil {
		return fmt.Errorf("恢复记忆整理日志失败: %w", err)
	}
	items := listAllTextMemories()

	// 唯一的整理输入：当前全部待合并组（Pending + 伙伴）。
	pending, partners := pendingCandidateSet(items)
	pending = clearOrphanPending(ctx, pending, partners)
	groupItems := pendingGroupItems(pending, partners)
	var clusters []memory.TextCluster
	if len(groupItems) > 0 {
		candidateIDs := make([]string, 0, len(groupItems))
		for _, item := range groupItems {
			candidateIDs = append(candidateIDs, item.ID)
		}
		var err error
		clusters, _, err = memory.ClusterTextCandidates(ctx, "", candidateIDs, memoryClusterThreshold)
		if err != nil {
			return fmt.Errorf("待合并组聚类失败: %w", err)
		}
	}

	batches, reason := planPendingBatches(groupItems, clusters)
	if len(batches) == 0 {
		r.data = memoryRunData{PlanFile: memoryPlanFile, Reason: reason}
		logger.Info(Module, "[记忆整理] 本次不整理：%s（待处理 %d 条、伙伴 %d 条）", reason, len(pending), len(partners))
		return nil
	}
	r.data = memoryRunData{PlanFile: memoryPlanFile, Total: len(batches)}
	plan := memoryPlan{Version: memoryPlanVersion, ID: fmt.Sprintf("mem-%d", time.Now().UnixNano()), Batches: batches}
	if err := saveMemoryPlan(plan); err != nil {
		return err
	}
	r.plan = &plan
	grouped := 0
	for _, batch := range batches {
		grouped += len(batch.Items)
	}
	logger.Info(Module, "[记忆整理] 待合并 %d 条（伙伴 %d 条）聚类出 %d 个组（共 %d 条），分 %d 批逐组处理",
		len(pending), len(partners), len(batches), grouped, len(batches))
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
	if plan.Version != memoryPlanVersion {
		return nil, fmt.Errorf("记忆整理计划版本过旧（%d != %d）", plan.Version, memoryPlanVersion)
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

// processMemoryBatch 整理一个待合并组。
//
// 保守处理：模型失败/空结果/没有实际合并/向量校验不通过/归档失败/替换失败时
// 都只跳过本组，绝不删除旧条目；该组保持待处理状态等待下一轮重试，
// 不存在基于条数或时间的强制淘汰兜底。
// 落库交给引擎 ReplaceTexts：先写新条目，校验通过后才删旧条目；失败自动回滚。
func processMemoryBatch(ctx context.Context, planID string, index int, batch memoryBatch) error {
	batchID := fmt.Sprintf("%s-%03d", planID, index)
	newTexts, err := mergeBatch(ctx, batch)
	if err != nil {
		logger.Warn(Module, "[记忆整理] 批次 %s 合并模型不可用，保留原 %d 条记忆不动: %v", batchID, len(batch.Items), err)
		return nil
	}
	if len(newTexts) == 0 {
		logger.Warn(Module, "[记忆整理] 批次 %s 整理结果为空，保留原 %d 条记忆不动", batchID, len(batch.Items))
		return nil
	}
	oldTexts := batchContents(batch)
	if sameTextSet(oldTexts, newTexts) {
		logger.Info(Module, "[记忆整理] 批次 %s 没有可合并的相似内容，%d 条原样保留", batchID, len(batch.Items))
		return nil
	}
	oldIDs := make([]string, 0, len(batch.Items))
	for _, item := range batch.Items {
		oldIDs = append(oldIDs, item.ID)
	}
	// 嵌入模型校验：新条目不能跑偏，旧条目不能被丢事实；不通过就保留原条目。
	if verifyErr := verifyHook(ctx, oldIDs, newTexts, memoryVerifyThreshold); verifyErr != nil {
		logger.Warn(Module, "[记忆整理] 批次 %s 向量校验未通过，保留原 %d 条记忆不动: %v", batchID, len(batch.Items), verifyErr)
		return nil
	}
	// 先归档旧条目再替换：归档失败就不删旧，避免淘汰后无法追溯。
	if archiveErr := archiveMemoryBatch(batchID, batch); archiveErr != nil {
		logger.Warn(Module, "[记忆整理] 批次 %s 归档旧条目失败，保留原记忆不动: %v", batchID, archiveErr)
		return nil
	}
	journal := memoryJournal{
		BatchID:     batchID,
		Status:      "prepared",
		OldEntries:  batch.Items,
		NewTexts:    newTexts,
		BeforeCount: len(batch.Items),
		UpdatedAt:   time.Now().Unix(),
	}
	if err := saveJournal(journal); err != nil {
		logger.Warn(Module, "[记忆整理] 批次 %s 写入意图日志失败，保留原记忆不动: %v", batchID, err)
		return nil
	}
	result, err := applyReplace(ctx, &journal)
	if err != nil {
		// 替换失败已由引擎回滚（旧条目全部保留），本组留待下一轮重试。
		return nil
	}
	journal.Status = "done"
	journal.AfterCount = len(result.CreatedIDs) + len(result.ReusedIDs)
	journal.UpdatedAt = time.Now().Unix()
	// 保留 done 日志：若紧接着进程崩溃（Index 尚未持久化），下次 recoverBatch 见到
	// done 会直接清理并跳过本批，避免对同一批旧条目重复调用模型、重复写入整理结果。
	if err := saveJournal(journal); err != nil {
		logger.Warn(Module, "[记忆整理] 批次 %s 写入完成日志失败（结果已落库）: %v", batchID, err)
	}
	logger.Info(Module, "[记忆整理] 批次 %s 完成：旧 %d 条 → 新 %d 条（新建 %d、复用 %d、删除 %d）",
		batchID, len(batch.Items), journal.AfterCount, len(result.CreatedIDs), len(result.ReusedIDs), len(result.DeletedIDs))
	return nil
}

// applyReplace 调用引擎层的「先写后删」事务替换，并把校验结果记入日志。
// 引擎会先快照旧条目，新文本全部落库并校验通过后才删除旧条目；
// 任一步失败都会回滚（删除本次新建、恢复旧条目），因此旧记忆不会因失败而消失。
func applyReplace(ctx context.Context, journal *memoryJournal) (memory.ReplaceResult, error) {
	oldIDs := make([]string, 0, len(journal.OldEntries))
	for _, item := range journal.OldEntries {
		oldIDs = append(oldIDs, item.ID)
	}
	result, err := replaceHook(ctx, oldIDs, journal.NewTexts, memory.ReplaceOptions{
		Role:   "assistant",
		Source: memoryReplaceSource,
	})
	journal.WrittenIDs = append([]string(nil), result.CreatedIDs...)
	journal.ReusedIDs = append([]string(nil), result.ReusedIDs...)
	journal.DeletedIDs = append([]string(nil), result.DeletedIDs...)
	journal.UpdatedAt = time.Now().Unix()
	if err != nil {
		journal.Status = "failed"
		journal.Error = err.Error()
		_ = saveJournal(*journal)
		logger.Warn(Module, "[记忆整理] 批次 %s 替换失败，旧记忆已保留: %v", journal.BatchID, err)
		return result, err
	}
	journal.Status = "replaced"
	journal.Error = ""
	_ = saveJournal(*journal)
	return result, nil
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
	if err := recoverJournal(ctx, journal); err != nil {
		return false, err
	}
	deleteJournal(batchID)
	return true, nil
}

// recoverJournal 重放一个未完成的批次。
//
// 幂等：新文本按「完全同文复用」落库，旧条目缺失按已删除处理；
// 结果为空时直接丢弃日志，绝不执行任何删除。
func recoverJournal(ctx context.Context, journal *memoryJournal) error {
	if len(journal.NewTexts) == 0 {
		logger.Warn(Module, "[记忆整理] 批次 %s 日志中待写入内容为空，保留旧记忆并清理日志", journal.BatchID)
		return nil
	}
	_, err := applyReplace(ctx, journal)
	return err
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
		if err := recoverJournal(ctx, journal); err != nil {
			logger.Warn(Module, "[记忆整理] 恢复批次 %s 失败（旧记忆保持不动）: %v", journal.BatchID, err)
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
