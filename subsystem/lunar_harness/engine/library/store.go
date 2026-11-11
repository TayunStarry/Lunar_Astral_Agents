package library

// store.go 编辑状态中心：把 AI 文件操作写入变更库，并广播一条同步信号。
//
// 数据方向是单向的：文件编辑智能体每完成一次操作就上报一次，资源库只读取与广播，
// 不接受任何正文修改。数据库（library.db）是变更记录的唯一存储，进程内不再保留
// 用于展示的内存副本；前端收到同步信号后自行回库拉取最新状态。

import (
	"database/sql"
	"strconv"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// SetBroadcast 注入 WebSocket 结构化广播（由装配层调用；nil 表示不广播）。
func SetBroadcast(fn func(msgType string, data any)) {
	broadcastMu.Lock()
	broadcast = fn
	broadcastMu.Unlock()
}

// PublishEdit 上报一次文件操作（线程安全、幂等）：落库后只广播同步信号。
func PublishEdit(notice EditNotice) {
	path := normalizeRel(notice.Path)
	if path == "" || rootOf(path) == "" {
		return
	}
	at := notice.At
	if at.IsZero() {
		at = time.Now()
	}
	conn, err := ensureDB()
	if err != nil {
		logger.Warn(Module, "变更库不可用，操作未记录: %v", err)
		return
	}
	seq, err := persistEdit(conn, path, notice, at)
	if err != nil {
		logger.Warn(Module, "写入变更记录失败: %v", err)
		return
	}
	if seq%cleanupEverySeq == 0 {
		if err := cleanupStore(conn); err != nil {
			logger.Warn(Module, "变更记录保留策略清理失败: %v", err)
		}
	}
	broadcastSync(seq, path, notice.Op)
}

// persistEdit 在一个事务内写入记录、更新文件状态并按文件裁剪历史。
func persistEdit(conn *sql.DB, path string, notice EditNotice, at time.Time) (uint64, error) {
	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	seq, err := nextSeq(tx)
	if err != nil {
		return 0, err
	}
	state, err := loadState(tx, path)
	if err != nil {
		return 0, err
	}
	if state == nil {
		state = &FileState{Path: path, Status: StatusNormal}
	}
	// 运行失败状态只在「运行」操作里更新：一次失败保持红色，直到下次运行成功。
	if notice.Op == "run" {
		state.hasRun = true
		state.runOK = !notice.Failed
		state.Failed = notice.Failed
	}
	if notice.Handle != "" {
		state.Handle = notice.Handle
	}
	state.Op = notice.Op
	state.At = at.Unix()
	if notice.Op != "run" {
		merged, mergeErr := coalesceRecord(tx, path, notice, at, seq)
		if mergeErr != nil {
			return 0, mergeErr
		}
		if !merged {
			if err := insertRecord(tx, path, notice, at, seq); err != nil {
				return 0, err
			}
		}
		if err := pruneRecords(tx, path); err != nil {
			return 0, err
		}
	}
	state.lastSeq = seq
	state.Status = statusOf(state)
	if err := saveState(tx, state); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return seq, nil
}

// nextSeq 读取并递增全局序号（增量同步与版本号的唯一来源）。
func nextSeq(tx *sql.Tx) (uint64, error) {
	var raw string
	err := tx.QueryRow(`SELECT value FROM `+metaTable+` WHERE key = ?`, metaSeqKey).Scan(&raw)
	if err == sql.ErrNoRows {
		raw = "0"
	} else if err != nil {
		return 0, err
	}
	current, _ := strconv.ParseUint(raw, 10, 64)
	next := current + 1
	_, err = tx.Exec(`INSERT INTO `+metaTable+` (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaSeqKey, strconv.FormatUint(next, 10))
	return next, err
}

// loadState 读取单个文件的当前状态；不存在时返回 nil。
func loadState(tx *sql.Tx, path string) (*FileState, error) {
	var state FileState
	var failed, hasRun, runOK int
	var lastSeq int64
	err := tx.QueryRow(`SELECT status, op, failed, handle, at, has_run, run_ok, last_seq
		FROM `+fileStateTable+` WHERE path = ?`, path).Scan(
		&state.Status, &state.Op, &failed, &state.Handle, &state.At, &hasRun, &runOK, &lastSeq)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state.Path = path
	state.Failed = failed != 0
	state.hasRun = hasRun != 0
	state.runOK = runOK != 0
	state.lastSeq = uint64(lastSeq)
	return &state, nil
}

// saveState 写入（或覆盖）单个文件的当前状态。
func saveState(tx *sql.Tx, state *FileState) error {
	_, err := tx.Exec(`INSERT INTO `+fileStateTable+` (
		path, status, op, failed, handle, at, has_run, run_ok, last_seq
	) VALUES (?,?,?,?,?,?,?,?,?)
	ON CONFLICT(path) DO UPDATE SET
		status = excluded.status,
		op = excluded.op,
		failed = excluded.failed,
		handle = excluded.handle,
		at = excluded.at,
		has_run = excluded.has_run,
		run_ok = excluded.run_ok,
		last_seq = excluded.last_seq`,
		state.Path, state.Status, state.Op, boolInt(state.Failed), state.Handle,
		state.At, boolInt(state.hasRun), boolInt(state.runOK), int64(state.lastSeq))
	return err
}

// insertRecord 写入一条内容级操作记录。
func insertRecord(tx *sql.Tx, path string, notice EditNotice, at time.Time, seq uint64) error {
	_, err := tx.Exec(`INSERT INTO `+editTable+` (
		seq, path, op, added, removed, start_line, lines, handle, failed, at, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		int64(seq), path, notice.Op, textutil.TruncateRunesTrimmed(notice.Added, maxSnippetRunes),
		textutil.TruncateRunesTrimmed(notice.Removed, maxSnippetRunes), notice.StartLine, notice.Lines,
		notice.Handle, boolInt(notice.Failed), at.Unix(), at.Unix())
	return err
}

// coalesceRecord 把窗口内无正文变化的同类操作合并到上一条记录，避免重复读取刷屏。
func coalesceRecord(tx *sql.Tx, path string, notice EditNotice, at time.Time, seq uint64) (bool, error) {
	if notice.Added != "" || notice.Removed != "" {
		return false, nil
	}
	var id, lastAt int64
	err := tx.QueryRow(`SELECT id, at FROM `+editTable+`
		WHERE path = ? AND op = ? AND added = '' AND removed = ''
			AND start_line = ? AND lines = ? AND failed = ?
		ORDER BY seq DESC LIMIT 1`,
		path, notice.Op, notice.StartLine, notice.Lines, boolInt(notice.Failed)).Scan(&id, &lastAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if at.Unix()-lastAt > coalesceWindowSec {
		return false, nil
	}
	_, err = tx.Exec(`UPDATE `+editTable+` SET seq = ?, at = ?, handle = ? WHERE id = ?`,
		int64(seq), at.Unix(), notice.Handle, id)
	return err == nil, err
}

// pruneRecords 只保留单个文件最近 maxRecordsPerFile 条记录。
func pruneRecords(tx *sql.Tx, path string) error {
	_, err := tx.Exec(`DELETE FROM `+editTable+` WHERE path = ? AND id NOT IN (
		SELECT id FROM `+editTable+` WHERE path = ? ORDER BY seq DESC LIMIT ?)`,
		path, path, maxRecordsPerFile)
	return err
}

// broadcastSync 广播同步信号：只携带版本与变更路径，前端据此回库拉取。
func broadcastSync(seq uint64, path, op string) {
	broadcastMu.RLock()
	fn := broadcast
	broadcastMu.RUnlock()
	if fn == nil {
		return
	}
	fn("library_sync", map[string]any{"version": seq, "path": path, "op": op})
}
