package fileflow

// delta_store.go 内容级增量记录的 SQLite 存储：建表、写入、按流程查询、保留策略清理。
//
// 库路径 = config.LocalDir + DeltaDirRel + DeltaDBFile，即 local_data/database/fileflow/flow_ops.db；
// 不从 engine/config 新增常量，避免与目录布局整理任务冲突。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	_ "github.com/mattn/go-sqlite3"
)

// DeltaDBPath 返回增量记录库的绝对路径（供报告与排查）。
func DeltaDBPath() string {
	return filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(DeltaDirRel), DeltaDBFile)
}

// deltaConn 打开（或复用）增量记录库；LocalDir 变化时自动重开。
func deltaConn() (*sql.DB, error) {
	deltaStoreMu.Lock()
	defer deltaStoreMu.Unlock()

	want := DeltaDBPath()
	if deltaDB != nil && deltaDBPath == want {
		return deltaDB, nil
	}
	if deltaDB != nil {
		_ = deltaDB.Close()
		deltaDB = nil
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		return nil, fmt.Errorf("创建增量记录目录失败: %w", err)
	}
	db, err := sql.Open("sqlite3", want+"?_busy_timeout=10000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("连接增量记录库失败: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("测试增量记录库连接失败: %w", err)
	}
	for _, stmt := range deltaSchema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("增量记录库建表失败: %w", err)
		}
	}
	deltaDB = db
	deltaDBPath = want
	logger.Info("文件操作", "增量记录库初始化完成: %s", want)
	return db, nil
}

// CloseDeltaStore 关闭增量记录库（测试隔离与进程清理使用）。
func CloseDeltaStore() error {
	deltaStoreMu.Lock()
	defer deltaStoreMu.Unlock()
	if deltaDB == nil {
		return nil
	}
	err := deltaDB.Close()
	deltaDB = nil
	deltaDBPath = ""
	return err
}

// insertOp 写入一条操作元数据与全部内容增量（同一事务）。
func insertOp(input OpInput, beforeHash, afterHash string, beforeSize, afterSize int64, deltas []Delta) error {
	db, err := deltaConn()
	if err != nil {
		return err
	}
	reviewStatus, reviewDetail, rollbackOK, rollbackError, status := reviewSummary(input.Review)
	now := time.Now().Unix()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启增量记录事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO `+deltaOpTable+` (
		flow_id, seq, step, mode, op, delta_kind, target, path_rel, path_abs, agent, tool, snapshot,
		before_sha256, after_sha256, before_size, after_size,
		review_status, review_detail, rollback_ok, rollback_error, status, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		input.FlowID, input.Seq, input.Step, string(input.Mode), input.Op, summarizeKinds(deltas), input.Target,
		input.PathRel, input.PathAbs, input.Agent, input.Tool, input.Snapshot.Dir,
		beforeHash, afterHash, beforeSize, afterSize,
		reviewStatus, reviewDetail, boolInt(rollbackOK), rollbackError, status, now)
	if err != nil {
		return fmt.Errorf("写入操作元数据失败: %w", err)
	}
	opID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("读取操作记录主键失败: %w", err)
	}
	for _, delta := range deltas {
		if _, err := tx.Exec(`INSERT INTO `+deltaTable+` (
			op_id, flow_id, seq, kind, path_rel, path_abs, source_rel, locator, line_from, line_to,
			old_text, new_text, old_hash, new_hash, old_size, new_size,
			binary, binary_note, truncated, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			opID, input.FlowID, input.Seq, delta.Kind, delta.PathRel, delta.PathAbs, delta.SourceRel,
			delta.Locator, delta.LineFrom, delta.LineTo, delta.OldText, delta.NewText,
			delta.OldHash, delta.NewHash, delta.OldSize, delta.NewSize,
			boolInt(delta.Binary), delta.BinaryNote, boolInt(delta.Truncated), now); err != nil {
			return fmt.Errorf("写入内容增量失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交增量记录失败: %w", err)
	}
	return nil
}

// reviewSummary 把审查结果翻译为存储字段。
func reviewSummary(review Review) (status, detail string, rollbackOK bool, rollbackError, opStatus string) {
	detail = review.Summary()
	opStatus = OpStatusApplied
	switch {
	case review.OK:
		status = ReviewPassed
	case review.RolledBack:
		status = ReviewRolledBack
		rollbackOK = true
		opStatus = OpStatusRolledBack
	case review.RollbackError != "":
		status = ReviewFailed
		rollbackError = review.RollbackError
		opStatus = OpStatusRollbackFailed
	default:
		status = ReviewFailed
	}
	return status, detail, rollbackOK, rollbackError, opStatus
}

// RecentOps 按时间倒序取最近 limit 条操作，返回时恢复为时间正序；含内容增量。
func RecentOps(limit int) ([]OpRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	db, err := deltaConn()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id FROM `+deltaOpTable+` ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近操作失败: %w", err)
	}
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		ids = append(ids, id)
	}
	rows.Close()
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return loadOpsByIDs(db, ids)
}

// FlowOps 按流程 ID 取全部操作记录（按 seq 正序），含内容增量。
func FlowOps(flowID string) ([]OpRecord, error) {
	db, err := deltaConn()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id FROM `+deltaOpTable+` WHERE flow_id = ? ORDER BY seq ASC, id ASC`, flowID)
	if err != nil {
		return nil, fmt.Errorf("查询流程操作失败: %w", err)
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		ids = append(ids, id)
	}
	rows.Close()
	return loadOpsByIDs(db, ids)
}

// loadOpsByIDs 批量装载操作元数据与内容增量。
func loadOpsByIDs(db *sql.DB, ids []int64) ([]OpRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ops := make([]OpRecord, 0, len(ids))
	for _, id := range ids {
		record, err := loadOp(db, id)
		if err != nil {
			return nil, err
		}
		ops = append(ops, record)
	}
	return ops, nil
}

// loadOp 读取一条操作记录及其增量。
func loadOp(db *sql.DB, id int64) (OpRecord, error) {
	var record OpRecord
	var mode string
	var rollbackOK int
	err := db.QueryRow(`SELECT
		id, flow_id, seq, step, mode, op, delta_kind, target, path_rel, path_abs, agent, tool, snapshot,
		before_sha256, after_sha256, before_size, after_size,
		review_status, review_detail, rollback_ok, rollback_error, status, created_at
		FROM `+deltaOpTable+` WHERE id = ?`, id).Scan(
		&record.ID, &record.FlowID, &record.Seq, &record.Step, &mode, &record.Op, &record.DeltaKind, &record.Target,
		&record.PathRel, &record.PathAbs, &record.Agent, &record.Tool, &record.Snapshot,
		&record.BeforeSHA256, &record.AfterSHA256, &record.BeforeSize, &record.AfterSize,
		&record.ReviewStatus, &record.ReviewDetail, &rollbackOK, &record.RollbackError,
		&record.Status, &record.At)
	if err != nil {
		return OpRecord{}, fmt.Errorf("读取操作记录失败: %w", err)
	}
	record.Mode = Mode(mode)
	record.RollbackOK = rollbackOK != 0
	record.Deltas, err = loadDeltas(db, id)
	if err != nil {
		return OpRecord{}, err
	}
	return record, nil
}

// loadDeltas 读取一条操作的全部内容增量。
func loadDeltas(db *sql.DB, opID int64) ([]Delta, error) {
	rows, err := db.Query(`SELECT
		kind, path_rel, path_abs, source_rel, locator, line_from, line_to,
		old_text, new_text, old_hash, new_hash, old_size, new_size,
		binary, binary_note, truncated
		FROM `+deltaTable+` WHERE op_id = ? ORDER BY id ASC`, opID)
	if err != nil {
		return nil, fmt.Errorf("读取内容增量失败: %w", err)
	}
	defer rows.Close()
	out := make([]Delta, 0)
	for rows.Next() {
		var delta Delta
		var binary, truncated int
		if scanErr := rows.Scan(
			&delta.Kind, &delta.PathRel, &delta.PathAbs, &delta.SourceRel, &delta.Locator,
			&delta.LineFrom, &delta.LineTo, &delta.OldText, &delta.NewText,
			&delta.OldHash, &delta.NewHash, &delta.OldSize, &delta.NewSize,
			&binary, &delta.BinaryNote, &truncated); scanErr != nil {
			return nil, scanErr
		}
		delta.Binary = binary != 0
		delta.Truncated = truncated != 0
		out = append(out, delta)
	}
	return out, rows.Err()
}

// markOpStatus 更新操作的回退状态。
func markOpStatus(opID int64, status string, ok bool, rollbackError string) error {
	db, err := deltaConn()
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE `+deltaOpTable+` SET status = ?, rollback_ok = ?, rollback_error = ? WHERE id = ?`,
		status, boolInt(ok), rollbackError, opID)
	if err != nil {
		return fmt.Errorf("更新回退状态失败: %w", err)
	}
	return nil
}

// insertRollback 记录一次按流程回退尝试。
func insertRollback(flowID string, targetSeq, fromSeq int, ok bool, detail string) {
	db, err := deltaConn()
	if err != nil {
		return
	}
	_, _ = db.Exec(`INSERT INTO `+deltaRollbackTable+` (flow_id, target_seq, from_seq, ok, detail, created_at)
		VALUES (?,?,?,?,?,?)`, flowID, targetSeq, fromSeq, boolInt(ok), detail, time.Now().Unix())
}

// CleanupDeltas 按流程数与时间清理增量记录；keepFlows <= 0 用默认值，maxAge <= 0 用默认值。
// 删除整条流程时同时删除该流程的操作与增量（回退记录保留，作为审计线索）。
func CleanupDeltas(keepFlows int, maxAge time.Duration) error {
	if keepFlows <= 0 {
		keepFlows = maxDeltaFlows
	}
	if maxAge <= 0 {
		maxAge = maxDeltaAgeDays * 24 * time.Hour
	}
	db, err := deltaConn()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge).Unix()

	// 超出保留数量的最旧流程
	rows, err := db.Query(`SELECT flow_id, MAX(created_at) AS last_at FROM ` + deltaOpTable + `
		GROUP BY flow_id ORDER BY last_at DESC`)
	if err != nil {
		return fmt.Errorf("统计流程保留情况失败: %w", err)
	}
	stale := make([]string, 0)
	seen := 0
	for rows.Next() {
		var flowID string
		var lastAt int64
		if scanErr := rows.Scan(&flowID, &lastAt); scanErr != nil {
			rows.Close()
			return scanErr
		}
		seen++
		if seen > keepFlows || lastAt < cutoff {
			stale = append(stale, flowID)
		}
	}
	rows.Close()

	for _, flowID := range stale {
		tx, beginErr := db.Begin()
		if beginErr != nil {
			return beginErr
		}
		_, delDeltas := tx.Exec(`DELETE FROM `+deltaTable+` WHERE flow_id = ?`, flowID)
		_, delOps := tx.Exec(`DELETE FROM `+deltaOpTable+` WHERE flow_id = ?`, flowID)
		if delDeltas != nil || delOps != nil || tx.Commit() != nil {
			_ = tx.Rollback()
			logger.Warn("文件操作", "清理流程增量失败: %s", flowID)
		}
	}
	// 兜底：删除超过保留时间且已无可关联流程的孤立增量
	_, _ = db.Exec(`DELETE FROM `+deltaTable+` WHERE created_at < ? AND op_id NOT IN (SELECT id FROM `+deltaOpTable+`)`, cutoff)
	return nil
}

// summarizeKinds 汇总一条操作的内容增量语义类型（多种时取第一条并标记 mixed）。
func summarizeKinds(deltas []Delta) string {
	if len(deltas) == 0 {
		return ""
	}
	kind := deltas[0].Kind
	for _, delta := range deltas[1:] {
		if delta.Kind != kind {
			return kind + "+mixed"
		}
	}
	return kind
}

// boolInt 布尔转 SQLite 整数。
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
