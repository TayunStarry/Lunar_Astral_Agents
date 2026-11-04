package fileflow

// change_store.go change 表：文件操作内容级增量的可见窗口。
//
// 设计取舍：
//   - 每个文件保留最新 maxChangeWindow 次操作对应的差异块，更久远的操作合并进 change_baselines（原始基准）；
//   - change 行通过 op_id 指向 flow_ops，flow_ops / flow_deltas 仍保留完整历史，
//     因此「合并」只影响提示词可见窗口，不会丢失 RollbackTo 的回退能力；
//   - 回溯上限仍是 CleanupDeltas 的流程保留策略，与窗口无关。

import (
	"database/sql"
	"fmt"
)

// insertChanges 把一次操作的内容级增量写入 change 表，并按文件裁剪可见窗口。
// 空路径（差异块数超限等占位增量）与内容未变化的条目不入窗口。
func insertChanges(tx *sql.Tx, callID string, opID int64, flowID string, seq int, deltas []Delta, now int64) error {
	touched := make(map[string]bool)
	for _, delta := range deltas {
		if delta.PathRel == "" {
			continue
		}
		if !delta.Binary && delta.OldHash == delta.NewHash {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO `+changeTable+` (
			op_id, call_id, flow_id, seq, kind, path_rel, path_abs, source_rel, locator,
			line_from, line_to, old_text, new_text, old_hash, new_hash, old_size, new_size,
			binary, binary_note, truncated, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			opID, callID, flowID, seq, delta.Kind, delta.PathRel, delta.PathAbs, delta.SourceRel,
			delta.Locator, delta.LineFrom, delta.LineTo, delta.OldText, delta.NewText,
			delta.OldHash, delta.NewHash, delta.OldSize, delta.NewSize,
			boolInt(delta.Binary), delta.BinaryNote, boolInt(delta.Truncated), now); err != nil {
			return fmt.Errorf("写入变更窗口失败: %w", err)
		}
		touched[delta.PathRel] = true
	}
	for path := range touched {
		if err := pruneChangePath(tx, path); err != nil {
			return err
		}
	}
	return nil
}

// pruneChangePath 按操作次数裁剪单个文件的可见窗口：仅保留最新 maxChangeWindow 次操作，
// 更早的操作聚合进原始基准后从 change 表移除（flow_ops / flow_deltas 不受影响）。
func pruneChangePath(tx *sql.Tx, pathRel string) error {
	rows, err := tx.Query(`SELECT DISTINCT op_id FROM `+changeTable+` WHERE path_rel = ? ORDER BY op_id ASC`, pathRel)
	if err != nil {
		return fmt.Errorf("统计变更窗口失败: %w", err)
	}
	opIDs := make([]int64, 0)
	for rows.Next() {
		var opID int64
		if scanErr := rows.Scan(&opID); scanErr != nil {
			rows.Close()
			return scanErr
		}
		opIDs = append(opIDs, opID)
	}
	rows.Close()
	if len(opIDs) <= maxChangeWindow {
		return nil
	}
	excess := opIDs[:len(opIDs)-maxChangeWindow]
	baseline, err := loadBaseline(tx, pathRel)
	if err != nil {
		return err
	}
	if baseline == nil {
		baseline = &ChangeBaseline{PathRel: pathRel}
	}
	for _, opID := range excess {
		if err := mergeOpIntoBaseline(tx, baseline, opID, pathRel); err != nil {
			return err
		}
	}
	return saveBaseline(tx, baseline)
}

// mergeOpIntoBaseline 把一次操作的差异块聚合进原始基准，并删除这些可见行。
func mergeOpIntoBaseline(tx *sql.Tx, baseline *ChangeBaseline, opID int64, pathRel string) error {
	rows, err := tx.Query(`SELECT flow_id, old_hash, new_hash, old_size, new_size, created_at
		FROM `+changeTable+` WHERE op_id = ? AND path_rel = ? ORDER BY id ASC`, opID, pathRel)
	if err != nil {
		return fmt.Errorf("读取待合并变更失败: %w", err)
	}
	var flowID, oldHash, newHash string
	var oldSize, newSize, createdAt int64
	first := true
	for rows.Next() {
		var rowFlowID, rowOldHash, rowNewHash string
		var rowOldSize, rowNewSize, rowAt int64
		if scanErr := rows.Scan(&rowFlowID, &rowOldHash, &rowNewHash, &rowOldSize, &rowNewSize, &rowAt); scanErr != nil {
			rows.Close()
			return scanErr
		}
		if first {
			flowID, oldHash, oldSize, createdAt = rowFlowID, rowOldHash, rowOldSize, rowAt
			first = false
		}
		newHash, newSize = rowNewHash, rowNewSize
	}
	rows.Close()
	if first {
		return nil
	}
	if baseline.MergedCount == 0 {
		baseline.FirstOpID = opID
		baseline.FirstFlowID = flowID
		baseline.BaseBeforeHash = oldHash
		baseline.BaseBeforeSize = oldSize
		baseline.FirstAt = createdAt
	}
	baseline.MergedCount++
	baseline.LastOpID = opID
	baseline.LatestAfterHash = newHash
	baseline.LatestAfterSize = newSize
	baseline.LastAt = createdAt
	_, _ = tx.Exec(`DELETE FROM `+changeTable+` WHERE op_id = ? AND path_rel = ?`, opID, pathRel)
	return nil
}

// ChangesOfPath 查询任意文件的可见变更窗口（按写入顺序正序）与合并后的原始基准。
func ChangesOfPath(pathRel string, limit int) ([]ChangeRecord, *ChangeBaseline, error) {
	db, err := deltaConn()
	if err != nil {
		return nil, nil, err
	}
	if limit <= 0 || limit > maxChangeWindow {
		limit = maxChangeWindow
	}
	rows, err := db.Query(`SELECT `+changeColumns+` FROM `+changeTable+`
		WHERE path_rel = ? ORDER BY id DESC LIMIT ?`, pathRel, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("查询文件变更失败: %w", err)
	}
	records, err := scanChanges(rows)
	if err != nil {
		return nil, nil, err
	}
	baseline, err := loadBaseline(db, pathRel)
	if err != nil {
		return nil, nil, err
	}
	return records, baseline, nil
}

// RecentChanges 按写入顺序倒序取最近 limit 条变更，返回时恢复为正序。
func RecentChanges(limit int) ([]ChangeRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	db, err := deltaConn()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT `+changeColumns+` FROM `+changeTable+`
		ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近变更失败: %w", err)
	}
	return scanChanges(rows)
}

// scanChanges 读取查询结果并按写入顺序正序返回。
func scanChanges(rows *sql.Rows) ([]ChangeRecord, error) {
	defer rows.Close()
	out := make([]ChangeRecord, 0)
	for rows.Next() {
		var record ChangeRecord
		var binary, truncated int
		if err := rows.Scan(
			&record.ID, &record.OpID, &record.CallID, &record.FlowID, &record.Seq, &record.Kind,
			&record.PathRel, &record.PathAbs, &record.SourceRel, &record.Locator,
			&record.LineFrom, &record.LineTo, &record.OldText, &record.NewText,
			&record.OldHash, &record.NewHash, &record.OldSize, &record.NewSize,
			&binary, &record.BinaryNote, &truncated, &record.CreatedAt); err != nil {
			return nil, err
		}
		record.Binary = binary != 0
		record.Truncated = truncated != 0
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// loadBaseline 读取单个文件的原始基准；不存在时返回 nil。
func loadBaseline(q queryer, pathRel string) (*ChangeBaseline, error) {
	var baseline ChangeBaseline
	err := q.QueryRow(`SELECT path_rel, merged_count, first_op_id, last_op_id, first_flow_id,
		base_before_hash, base_before_size, latest_after_hash, latest_after_size, first_at, last_at
		FROM `+changeBaselineTable+` WHERE path_rel = ?`, pathRel).Scan(
		&baseline.PathRel, &baseline.MergedCount, &baseline.FirstOpID, &baseline.LastOpID,
		&baseline.FirstFlowID, &baseline.BaseBeforeHash, &baseline.BaseBeforeSize,
		&baseline.LatestAfterHash, &baseline.LatestAfterSize, &baseline.FirstAt, &baseline.LastAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取变更基准失败: %w", err)
	}
	return &baseline, nil
}

// saveBaseline 写入（或覆盖）单个文件的原始基准。
func saveBaseline(tx *sql.Tx, baseline *ChangeBaseline) error {
	_, err := tx.Exec(`INSERT INTO `+changeBaselineTable+` (
		path_rel, merged_count, first_op_id, last_op_id, first_flow_id,
		base_before_hash, base_before_size, latest_after_hash, latest_after_size, first_at, last_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(path_rel) DO UPDATE SET
		merged_count = excluded.merged_count,
		first_op_id = excluded.first_op_id,
		last_op_id = excluded.last_op_id,
		first_flow_id = excluded.first_flow_id,
		base_before_hash = excluded.base_before_hash,
		base_before_size = excluded.base_before_size,
		latest_after_hash = excluded.latest_after_hash,
		latest_after_size = excluded.latest_after_size,
		first_at = excluded.first_at,
		last_at = excluded.last_at`,
		baseline.PathRel, baseline.MergedCount, baseline.FirstOpID, baseline.LastOpID, baseline.FirstFlowID,
		baseline.BaseBeforeHash, baseline.BaseBeforeSize, baseline.LatestAfterHash, baseline.LatestAfterSize,
		baseline.FirstAt, baseline.LastAt)
	if err != nil {
		return fmt.Errorf("写入变更基准失败: %w", err)
	}
	return nil
}
