package library

// query.go 变更库的读取入口：全量 / 增量状态快照与单文件历史分页。
//
// 所有读取都直接面向 library.db，重启后仍能返回历史记录。

import (
	"database/sql"
	"fmt"
	"strconv"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Snapshot 返回全部文件状态（兼容旧调用；数据库不可用时返回空集合）。
func Snapshot() EditSnapshot {
	snapshot, err := Edits(EditQuery{})
	if err != nil {
		logger.Warn(Module, "读取变更状态失败: %v", err)
		return EditSnapshot{Files: []FileState{}}
	}
	return snapshot
}

// Edits 按增量游标与分页参数读取文件状态。
func Edits(query EditQuery) (EditSnapshot, error) {
	conn, err := ensureDB()
	if err != nil {
		return EditSnapshot{}, err
	}
	current, err := currentVersion(conn)
	if err != nil {
		return EditSnapshot{}, err
	}
	reset := false
	if query.Since > current {
		// 游标超过当前版本：库被清空或重建，前端需要全量重读。
		query.Since = 0
		reset = true
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Limit < 0 {
		query.Limit = 0
	}
	if query.Limit > maxPageSize {
		query.Limit = maxPageSize
	}
	total, err := countStates(conn, query.Since)
	if err != nil {
		return EditSnapshot{}, err
	}
	files, err := loadStates(conn, query)
	if err != nil {
		return EditSnapshot{}, err
	}
	for i := range files {
		records, recordErr := loadRecentRecords(conn, files[i].Path, maxEditHistory)
		if recordErr != nil {
			return EditSnapshot{}, recordErr
		}
		files[i].Records = records
	}
	return EditSnapshot{
		Version: current,
		Files:   files,
		Total:   total,
		Offset:  query.Offset,
		Limit:   query.Limit,
		HasMore: query.Offset+len(files) < total,
		Reset:   reset,
	}, nil
}

// History 读取单个文件的历史记录分页（最新在前）。
func History(query HistoryQuery) (HistoryPage, error) {
	path := normalizeRel(query.Path)
	if path == "" || rootOf(path) == "" {
		return HistoryPage{}, fmt.Errorf("路径不在资源库范围内")
	}
	conn, err := ensureDB()
	if err != nil {
		return HistoryPage{}, err
	}
	if query.Limit <= 0 {
		query.Limit = defaultHistoryPageSize
	}
	if query.Limit > maxPageSize {
		query.Limit = maxPageSize
	}
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM `+editTable+` WHERE path = ?`, path).Scan(&total); err != nil {
		return HistoryPage{}, fmt.Errorf("统计历史记录失败: %w", err)
	}
	records, nextBefore, hasMore, err := loadHistoryRecords(conn, path, query.Before, query.Limit)
	if err != nil {
		return HistoryPage{}, err
	}
	return HistoryPage{
		Path:       path,
		Records:    records,
		Total:      total,
		NextBefore: nextBefore,
		HasMore:    hasMore,
	}, nil
}

// currentVersion 读取全局序号（增量同步与版本号）。
func currentVersion(conn *sql.DB) (uint64, error) {
	var raw string
	err := conn.QueryRow(`SELECT value FROM `+metaTable+` WHERE key = ?`, metaSeqKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读取变更版本失败: %w", err)
	}
	version, _ := strconv.ParseUint(raw, 10, 64)
	return version, nil
}

// countStates 统计满足增量条件的文件状态数量。
func countStates(conn *sql.DB, since uint64) (int, error) {
	query := `SELECT COUNT(*) FROM ` + fileStateTable
	args := make([]any, 0, 1)
	if since > 0 {
		query += ` WHERE last_seq > ?`
		args = append(args, int64(since))
	}
	var total int
	if err := conn.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计文件状态失败: %w", err)
	}
	return total, nil
}

// loadStates 按路径升序读取文件状态，支持增量与分页。
func loadStates(conn *sql.DB, query EditQuery) ([]FileState, error) {
	statement := `SELECT path, status, op, failed, handle, at, has_run, run_ok, last_seq
		FROM ` + fileStateTable
	args := make([]any, 0, 3)
	if query.Since > 0 {
		statement += ` WHERE last_seq > ?`
		args = append(args, int64(query.Since))
	}
	statement += ` ORDER BY path ASC`
	if query.Limit > 0 {
		statement += ` LIMIT ? OFFSET ?`
		args = append(args, query.Limit, query.Offset)
	}
	rows, err := conn.Query(statement, args...)
	if err != nil {
		return nil, fmt.Errorf("读取文件状态失败: %w", err)
	}
	defer rows.Close()
	files := make([]FileState, 0, 16)
	for rows.Next() {
		var state FileState
		var failed, hasRun, runOK int
		var lastSeq int64
		if err := rows.Scan(&state.Path, &state.Status, &state.Op, &failed, &state.Handle,
			&state.At, &hasRun, &runOK, &lastSeq); err != nil {
			return nil, err
		}
		state.Failed = failed != 0
		state.hasRun = hasRun != 0
		state.runOK = runOK != 0
		state.lastSeq = uint64(lastSeq)
		files = append(files, state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

// loadRecentRecords 读取单个文件最近 limit 条记录（按时间正序返回）。
func loadRecentRecords(conn *sql.DB, path string, limit int) ([]EditRecord, error) {
	rows, err := conn.Query(`SELECT op, added, removed, start_line, lines, at
		FROM `+editTable+` WHERE path = ? ORDER BY seq DESC LIMIT ?`, path, limit)
	if err != nil {
		return nil, fmt.Errorf("读取最近操作失败: %w", err)
	}
	defer rows.Close()
	records := make([]EditRecord, 0, limit)
	for rows.Next() {
		var record EditRecord
		if err := rows.Scan(&record.Op, &record.Added, &record.Removed,
			&record.StartLine, &record.Lines, &record.At); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}

// loadHistoryRecords 按游标读取单文件历史页，返回下一页游标与是否还有更早记录。
func loadHistoryRecords(conn *sql.DB, path string, before uint64, limit int) ([]EditRecord, uint64, bool, error) {
	statement := `SELECT seq, op, added, removed, start_line, lines, at FROM ` + editTable + ` WHERE path = ?`
	args := []any{path}
	if before > 0 {
		statement += ` AND seq < ?`
		args = append(args, int64(before))
	}
	statement += ` ORDER BY seq DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := conn.Query(statement, args...)
	if err != nil {
		return nil, 0, false, fmt.Errorf("读取历史记录失败: %w", err)
	}
	defer rows.Close()
	records := make([]EditRecord, 0, limit)
	seqs := make([]uint64, 0, limit)
	for rows.Next() {
		var record EditRecord
		var seq int64
		if err := rows.Scan(&seq, &record.Op, &record.Added, &record.Removed,
			&record.StartLine, &record.Lines, &record.At); err != nil {
			return nil, 0, false, err
		}
		records = append(records, record)
		seqs = append(seqs, uint64(seq))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
		seqs = seqs[:limit]
	}
	nextBefore := uint64(0)
	if len(seqs) > 0 {
		nextBefore = seqs[len(seqs)-1]
	}
	return records, nextBefore, hasMore, nil
}
