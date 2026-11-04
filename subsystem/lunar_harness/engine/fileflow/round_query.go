package fileflow

// round_query.go 跨轮编排态的读取：上一轮（上一个已完成调用）的步骤、参考资料、备忘录、错误与变更。

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// PrevRound 读取指定模式上一个已完成调用（running 不计入「上一轮」）的可见信息。
func PrevRound(mode Mode) (RoundSnapshot, error) {
	db, err := deltaConn()
	if err != nil {
		return RoundSnapshot{}, err
	}
	call, err := loadDoneCall(db, mode)
	if err != nil {
		return RoundSnapshot{}, err
	}
	if call == nil {
		return RoundSnapshot{Call: CallRecord{Mode: mode}}, nil
	}
	steps, err := loadRoundSteps(db, modeStepTable(mode), call.CallID)
	if err != nil {
		return RoundSnapshot{}, err
	}
	errs, err := loadRoundItems(db, modeErrorTable(mode), "message", call.CallID)
	if err != nil {
		return RoundSnapshot{}, err
	}
	changes, err := loadChangesByCall(db, call.CallID)
	if err != nil {
		return RoundSnapshot{}, err
	}
	snapshot := RoundSnapshot{Call: *call, Steps: steps, Errors: errs, Changes: changes}
	if mode == ModeCreator {
		if snapshot.References, err = loadRoundItems(db, referenceTable, "content", call.CallID); err != nil {
			return RoundSnapshot{}, err
		}
		if snapshot.Memos, err = loadRoundItems(db, memoTable, "content", call.CallID); err != nil {
			return RoundSnapshot{}, err
		}
	}
	return snapshot, nil
}

// loadDoneCall 读取指定模式最近一个已完成的调用；不存在时返回 nil。
func loadDoneCall(db *sql.DB, mode Mode) (*CallRecord, error) {
	var record CallRecord
	var modeText string
	err := db.QueryRow(`SELECT call_id, mode, status, summary, started_at, ended_at
		FROM `+callTable+` WHERE mode = ? AND status <> ? ORDER BY started_at DESC, call_id DESC LIMIT 1`,
		string(mode), CallStatusRunning).Scan(
		&record.CallID, &modeText, &record.Status, &record.Summary, &record.StartedAt, &record.EndedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取上一轮调用失败: %w", err)
	}
	record.Mode = Mode(modeText)
	return &record, nil
}

// loadRoundSteps 读取一次调用的步骤。
func loadRoundSteps(db *sql.DB, table, callID string) ([]RoundStep, error) {
	rows, err := db.Query(`SELECT ordinal, agent, task, expects FROM `+table+`
		WHERE call_id = ? ORDER BY ordinal ASC`, callID)
	if err != nil {
		return nil, fmt.Errorf("读取调用步骤失败: %w", err)
	}
	defer rows.Close()
	out := make([]RoundStep, 0)
	for rows.Next() {
		var step RoundStep
		var expects string
		if scanErr := rows.Scan(&step.Ordinal, &step.Agent, &step.Task, &expects); scanErr != nil {
			return nil, scanErr
		}
		if expects != "" {
			if decodeErr := json.Unmarshal([]byte(expects), &step.Expects); decodeErr != nil {
				return nil, fmt.Errorf("解析规划预期失败: %w", decodeErr)
			}
		}
		out = append(out, step)
	}
	return out, rows.Err()
}

// loadRoundItems 读取一次调用的文本条目。
func loadRoundItems(db *sql.DB, table, column, callID string) ([]string, error) {
	query := `SELECT ` + column + ` FROM ` + tableExpr(table) + ` WHERE call_id = ? ORDER BY ordinal ASC`
	rows, err := db.Query(query, callID)
	if err != nil {
		return nil, fmt.Errorf("读取调用条目失败: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var item string
		if scanErr := rows.Scan(&item); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// loadChangesByCall 读取一次调用产生的全部可见变更。
func loadChangesByCall(db *sql.DB, callID string) ([]ChangeRecord, error) {
	if callID == "" {
		return nil, nil
	}
	rows, err := db.Query(`SELECT `+changeColumns+` FROM `+changeTable+`
		WHERE call_id = ? ORDER BY id ASC`, callID)
	if err != nil {
		return nil, fmt.Errorf("读取本轮变更失败: %w", err)
	}
	return scanChanges(rows)
}
