package fileflow

// journal_migrate.go 旧 JSON 创作日志的一次性导入（只读原文件、不删除）。
//
// 来源两个历史文件：
//   - database/life/creative_journal.json  → 自由创作模式
//   - database/life/creative_assigned.json → 对话者指定模式
//
// 留存规则与运行期一致：自由创作导入 steps_free / errors_free / references / memos；
// 指派模式只导入 steps_assign / errors_assign，参考资料与备忘录不落库。
// 幂等：store_meta 标记 + 表内唯一索引（call_id, ordinal / call_id, content）。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

// MigrateLegacyJournals 返回旧 JSON 日志的导入状态与行数（首次调用会触发导入）。
func MigrateLegacyJournals() (JournalMigrationReport, error) {
	db, err := deltaConn()
	if err != nil {
		return JournalMigrationReport{}, err
	}
	return journalMigrationReport(db)
}

// journalSources 返回两处旧 JSON 日志的来源定义。
func journalSources() []journalSource {
	return []journalSource{
		{PathRel: legacyJournalFile, Mode: ModeCreator},
		{PathRel: legacyAssignedJournalFile, Mode: ModeAssigned},
	}
}

// migrateLegacyJournals 逐个来源执行幂等导入；文件不存在时跳过且不写标记。
func migrateLegacyJournals(db *sql.DB) error {
	for _, source := range journalSources() {
		if value, ok, err := readMeta(db, legacyJournalMarker(source.Mode)); err != nil {
			return err
		} else if ok && value != "" {
			continue
		}
		journal, exists, err := readLegacyJournal(source.PathRel)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := importLegacyJournal(db, source, journal); err != nil {
			return err
		}
	}
	return nil
}

// readLegacyJournal 读取并解析旧 JSON 日志；不存在时返回 exists=false。
func readLegacyJournal(pathRel string) (legacyJournal, bool, error) {
	data, err := os.ReadFile(legacyJournalPath(pathRel))
	if os.IsNotExist(err) {
		return legacyJournal{}, false, nil
	}
	if err != nil {
		return legacyJournal{}, false, fmt.Errorf("读取旧日志失败 %s: %w", pathRel, err)
	}
	var journal legacyJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return legacyJournal{}, false, fmt.Errorf("解析旧日志失败 %s: %w", pathRel, err)
	}
	return journal, true, nil
}

// importLegacyJournal 把一份旧日志写入统一的调用轮次与对应模式的分表。
func importLegacyJournal(db *sql.DB, source journalSource, journal legacyJournal) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启旧日志导入事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	callID := legacyJournalCallID(source.Mode)
	at := journal.UpdatedAt
	if at <= 0 {
		at = time.Now().Unix()
	}
	summary := ""
	if source.Mode == ModeCreator {
		summary = journal.LastSummary
	}
	if _, err := tx.Exec(`INSERT INTO `+callTable+` (call_id, mode, status, summary, started_at, ended_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(call_id) DO UPDATE SET summary = excluded.summary, ended_at = excluded.ended_at`,
		callID, string(source.Mode), CallStatusDone, summary, at, at); err != nil {
		return fmt.Errorf("写入旧日志调用轮次失败: %w", err)
	}

	steps := make([]RoundStep, 0, len(journal.LastPlans))
	for index, plan := range journal.LastPlans {
		steps = append(steps, RoundStep{Ordinal: index, Agent: plan.Agent, Task: plan.Task, Expects: plan.Expect})
	}
	if err := appendRoundSteps(tx, modeStepTable(source.Mode), callID, steps, at); err != nil {
		return err
	}
	if source.Mode == ModeCreator {
		if err := appendRoundItems(tx, referenceTable, "content", callID, journal.References, at); err != nil {
			return err
		}
		if err := appendRoundItems(tx, memoTable, "content", callID, journal.Memos, at); err != nil {
			return err
		}
	}
	if err := appendRoundItems(tx, modeErrorTable(source.Mode), "message", callID, journal.Errors, at); err != nil {
		return err
	}
	if err := writeMeta(tx, legacyJournalMarker(source.Mode), "1"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交旧日志导入失败: %w", err)
	}
	return nil
}

// journalMigrationReport 汇总两处旧日志的存在性、导入标记与当前行数。
func journalMigrationReport(db *sql.DB) (JournalMigrationReport, error) {
	report := JournalMigrationReport{Files: make([]JournalFileReport, 0, 2)}
	for _, source := range journalSources() {
		item := JournalFileReport{Path: legacyJournalPath(source.PathRel), Mode: source.Mode, CallID: legacyJournalCallID(source.Mode)}
		if _, err := os.Stat(item.Path); err == nil {
			item.Exists = true
		} else if !os.IsNotExist(err) {
			return report, fmt.Errorf("检查旧日志失败 %s: %w", item.Path, err)
		}
		value, ok, err := readMeta(db, legacyJournalMarker(source.Mode))
		if err != nil {
			return report, err
		}
		if ok && value != "" {
			item.Skipped = true
			if item.Steps, err = countJournalRows(db, modeStepTable(source.Mode), item.CallID); err != nil {
				return report, err
			}
			if item.Errors, err = countJournalRows(db, modeErrorTable(source.Mode), item.CallID); err != nil {
				return report, err
			}
			if source.Mode == ModeCreator {
				if item.References, err = countJournalRows(db, referenceTable, item.CallID); err != nil {
					return report, err
				}
				if item.Memos, err = countJournalRows(db, memoTable, item.CallID); err != nil {
					return report, err
				}
			}
		}
		report.Files = append(report.Files, item)
	}
	return report, nil
}

// countJournalRows 统计某次合成调用在某张表中的行数。
func countJournalRows(db *sql.DB, table, callID string) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM `+tableExpr(table)+` WHERE call_id = ?`, callID).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计旧日志导入行数失败: %w", err)
	}
	return count, nil
}

// legacyJournalMarker 返回模式对应的导入标记键。
func legacyJournalMarker(mode Mode) string {
	if mode == ModeAssigned {
		return metaLegacyJournalAssignedKey
	}
	return metaLegacyJournalCreatorKey
}

// legacyJournalCallID 返回旧日志导入使用的合成调用 ID（固定值便于去重与查询）。
func legacyJournalCallID(mode Mode) string {
	return "legacy-journal-" + string(mode)
}

// legacyJournalPath 把相对路径展开到 LocalDir 下的绝对路径。
func legacyJournalPath(pathRel string) string {
	return filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(pathRel))
}

// readMeta 读取库级元数据。
func readMeta(q queryer, key string) (string, bool, error) {
	var value string
	err := q.QueryRow(`SELECT value FROM `+metaTable+` WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("读取库元数据失败: %w", err)
	}
	return value, true, nil
}

// writeMeta 写入库级元数据。
func writeMeta(tx *sql.Tx, key, value string) error {
	if _, err := tx.Exec(`INSERT OR REPLACE INTO `+metaTable+` (key, value) VALUES (?,?)`, key, value); err != nil {
		return fmt.Errorf("写入库元数据失败: %w", err)
	}
	return nil
}
