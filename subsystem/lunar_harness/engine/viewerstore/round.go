package viewerstore

// round.go 观影轮次的写入与 FIFO 归档。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SaveRound 写入一条观影轮次；超出上限按最旧归档后删除。
func SaveRound(round Round) error {
	round.ID = strings.TrimSpace(round.ID)
	if round.ID == "" {
		return fmt.Errorf("轮次 ID 为空")
	}
	if round.CreatedAt == 0 {
		round.CreatedAt = time.Now().Unix()
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO viewer_round (round_id, bvids, conclusion, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(round_id) DO UPDATE SET
			bvids=excluded.bvids, conclusion=excluded.conclusion, created_at=excluded.created_at`,
		round.ID, round.BVIDs, round.Conclusion, round.CreatedAt); err != nil {
		return fmt.Errorf("写入观影轮次失败: %w", err)
	}

	rows, err := tx.Query(`SELECT round_id, bvids, conclusion, created_at
		FROM viewer_round ORDER BY created_at ASC, round_id ASC`)
	if err != nil {
		return err
	}
	rounds := make([]Round, 0, RoundMax+1)
	for rows.Next() {
		var item Round
		if err := rows.Scan(&item.ID, &item.BVIDs, &item.Conclusion, &item.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		rounds = append(rounds, item)
	}
	rows.Close()
	if overflow := len(rounds) - RoundMax; overflow > 0 {
		for _, stale := range rounds[:overflow] {
			if err := archiveAndDeleteRound(tx, stale); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// archiveAndDeleteRound 先归档再删除一条轮次记录。
func archiveAndDeleteRound(tx *sql.Tx, round Round) error {
	payload, err := json.Marshal(round)
	if err != nil {
		return fmt.Errorf("归档轮次失败: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO viewer_archive (kind, payload, reason, archived_at)
		VALUES (?, ?, ?, ?)`, "round", string(payload), archiveRoundOverflow, time.Now().Unix()); err != nil {
		return fmt.Errorf("归档轮次失败: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM viewer_round WHERE round_id = ?`, round.ID); err != nil {
		return fmt.Errorf("淘汰轮次失败: %w", err)
	}
	return nil
}
