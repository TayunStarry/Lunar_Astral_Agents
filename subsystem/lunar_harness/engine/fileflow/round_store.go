package fileflow

// round_store.go 跨轮编排态的写入：调用轮次（calls）、步骤（steps_*）、错误（errors_*）、
// 参考资料（references）与备忘录（memos）。
//
// 留存规则：
//   - 自由创作模式（ModeCreator）写入 references / memos / steps_free / errors_free；
//   - 对话者指定模式（ModeAssigned）只写 steps_assign / errors_assign，参考资料与备忘录不落库。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// BeginCall 开始一轮调用并返回 call_id（一轮 = 一次外部任务输入到一次任务输出）。
func BeginCall(mode Mode) (string, error) {
	db, err := deltaConn()
	if err != nil {
		return "", err
	}
	callID := NewFlowID()
	if _, err := db.Exec(`INSERT INTO `+callTable+` (call_id, mode, status, summary, started_at, ended_at)
		VALUES (?,?,?,?,?,?)`, callID, string(mode), CallStatusRunning, "", time.Now().Unix(), 0); err != nil {
		return "", fmt.Errorf("创建调用轮次失败: %w", err)
	}
	if cleanupErr := CleanupRounds(0); cleanupErr != nil {
		return callID, cleanupErr
	}
	return callID, nil
}

// FinishRound 结束一轮调用：按模式写入步骤、错误与（自由创作专属的）参考资料与备忘录。
func FinishRound(callID string, mode Mode, status, summary string, steps []RoundStep, refs, memos, errs []string) error {
	if callID == "" {
		return nil
	}
	db, err := deltaConn()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启轮次归档事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	if err := appendRoundSteps(tx, modeStepTable(mode), callID, steps, now); err != nil {
		return err
	}
	if err := appendRoundItems(tx, modeErrorTable(mode), "message", callID, errs, now); err != nil {
		return err
	}
	if mode == ModeCreator {
		if err := appendRoundItems(tx, referenceTable, "content", callID, refs, now); err != nil {
			return err
		}
		if err := appendRoundItems(tx, memoTable, "content", callID, memos, now); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE `+callTable+` SET status = ?, summary = ?, ended_at = ? WHERE call_id = ?`,
			status, summary, now, callID)
	} else {
		_, err = tx.Exec(`UPDATE `+callTable+` SET status = ?, ended_at = ? WHERE call_id = ?`,
			status, now, callID)
	}
	if err != nil {
		return fmt.Errorf("更新调用轮次状态失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交轮次归档失败: %w", err)
	}
	return nil
}

// CleanupRounds 每种模式只保留最近 keep 轮调用；keep <= 0 时使用默认值。
// 清理只影响调用轮次及其步骤 / 错误 / 参考 / 备忘，文件变更窗口与回退记录不受影响。
func CleanupRounds(keep int) error {
	if keep <= 0 {
		keep = maxRoundCalls
	}
	db, err := deltaConn()
	if err != nil {
		return err
	}
	for _, mode := range []Mode{ModeCreator, ModeAssigned} {
		rows, err := db.Query(`SELECT call_id FROM `+callTable+`
			WHERE mode = ? ORDER BY started_at DESC, call_id DESC LIMIT -1 OFFSET ?`, string(mode), keep)
		if err != nil {
			return fmt.Errorf("统计轮次保留情况失败: %w", err)
		}
		stale := make([]string, 0)
		for rows.Next() {
			var callID string
			if scanErr := rows.Scan(&callID); scanErr != nil {
				rows.Close()
				return scanErr
			}
			stale = append(stale, callID)
		}
		rows.Close()
		for _, callID := range stale {
			if _, delErr := db.Exec(`DELETE FROM `+callTable+` WHERE call_id = ?`, callID); delErr != nil {
				return fmt.Errorf("清理调用轮次失败: %w", delErr)
			}
		}
	}
	return nil
}

// appendRoundSteps 写入步骤；expects 序列化为 JSON。
func appendRoundSteps(tx *sql.Tx, table, callID string, steps []RoundStep, now int64) error {
	for i, step := range steps {
		expects := ""
		if len(step.Expects) > 0 {
			raw, err := json.Marshal(step.Expects)
			if err != nil {
				return fmt.Errorf("序列化规划预期失败: %w", err)
			}
			expects = string(raw)
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO `+table+` (call_id, ordinal, agent, task, expects, created_at)
			VALUES (?,?,?,?,?,?)`, callID, i, step.Agent, step.Task, expects, now); err != nil {
			return fmt.Errorf("写入调用步骤失败: %w", err)
		}
	}
	return nil
}

// appendRoundItems 写入参考资料 / 备忘 / 错误等文本条目（参考资料与备忘按内容去重）。
func appendRoundItems(tx *sql.Tx, table, column, callID string, items []string, now int64) error {
	expr := tableExpr(table)
	for i, item := range items {
		if item == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO `+expr+` (call_id, ordinal, `+column+`, created_at)
			VALUES (?,?,?,?)`, callID, i, item, now); err != nil {
			return fmt.Errorf("写入调用条目失败: %w", err)
		}
	}
	return nil
}

// tableExpr 返回可安全用于 SQL 的表名（references 是关键字，需要加引号）。
func tableExpr(table string) string {
	if table == referenceTable {
		return `"` + table + `"`
	}
	return table
}

// modeStepTable 返回模式对应的步骤表名。
func modeStepTable(mode Mode) string {
	if mode == ModeAssigned {
		return stepAssignTable
	}
	return stepFreeTable
}

// modeErrorTable 返回模式对应的错误表名。
func modeErrorTable(mode Mode) string {
	if mode == ModeAssigned {
		return errorAssignTable
	}
	return errorFreeTable
}
