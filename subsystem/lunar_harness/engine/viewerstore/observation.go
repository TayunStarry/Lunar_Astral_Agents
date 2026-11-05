package viewerstore

// observation.go 近期观感的读写与 FIFO 归档。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SaveObservation 写入一条近期观感：同 BV 覆盖更新，超出上限按最旧归档后删除。
//
// 归档与删除在同一个事务里完成：归档失败会一起回滚，不会出现「删了没归档」。
func SaveObservation(obs Observation) error {
	obs.BV = strings.TrimSpace(obs.BV)
	if obs.BV == "" {
		return fmt.Errorf("观感缺少 BV 号")
	}
	if obs.UpdatedAt == 0 {
		obs.UpdatedAt = time.Now().Unix()
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

	if _, err := tx.Exec(`INSERT INTO viewer_observation
		(bv, title, url, topics, tone, conclusion, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(bv) DO UPDATE SET
			title=excluded.title, url=excluded.url, topics=excluded.topics,
			tone=excluded.tone, conclusion=excluded.conclusion, updated_at=excluded.updated_at`,
		obs.BV, obs.Title, obs.URL, obs.Topics, obs.Tone, obs.Conclusion, obs.UpdatedAt); err != nil {
		return fmt.Errorf("写入观感失败: %w", err)
	}

	observations, err := queryObservations(tx)
	if err != nil {
		return err
	}
	if overflow := len(observations) - ObservationMax; overflow > 0 {
		for _, stale := range observations[:overflow] {
			if err := archiveAndDeleteObservation(tx, stale); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// LoadObservations 按更新时间正序读取全部近期观感。
func LoadObservations() ([]Observation, error) {
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	return queryObservations(db)
}

// LoadObservationByBV 按 BV 号查一条观感。
func LoadObservationByBV(bv string) (Observation, bool, error) {
	db, err := openStore()
	if err != nil {
		return Observation{}, false, err
	}
	var obs Observation
	err = db.QueryRow(`SELECT bv, title, url, topics, tone, conclusion, updated_at
		FROM viewer_observation WHERE bv = ?`, strings.TrimSpace(bv)).
		Scan(&obs.BV, &obs.Title, &obs.URL, &obs.Topics, &obs.Tone, &obs.Conclusion, &obs.UpdatedAt)
	if err == sql.ErrNoRows {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	return obs, true, nil
}

// queryer 兼容 *sql.DB 与 *sql.Tx。
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func queryObservations(q queryer) ([]Observation, error) {
	rows, err := q.Query(`SELECT bv, title, url, topics, tone, conclusion, updated_at
		FROM viewer_observation ORDER BY updated_at ASC, bv ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Observation, 0, ObservationMax+1)
	for rows.Next() {
		var obs Observation
		if err := rows.Scan(&obs.BV, &obs.Title, &obs.URL, &obs.Topics, &obs.Tone, &obs.Conclusion, &obs.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, obs)
	}
	return out, rows.Err()
}

// archiveAndDeleteObservation 先归档再删除一条观感（同一事务内）。
func archiveAndDeleteObservation(tx *sql.Tx, obs Observation) error {
	payload, err := json.Marshal(obs)
	if err != nil {
		return fmt.Errorf("归档观感失败: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO viewer_archive (kind, payload, reason, archived_at)
		VALUES (?, ?, ?, ?)`, "observation", string(payload), archiveObservationOverflow, time.Now().Unix()); err != nil {
		return fmt.Errorf("归档观感失败: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM viewer_observation WHERE bv = ?`, obs.BV); err != nil {
		return fmt.Errorf("淘汰观感失败: %w", err)
	}
	return nil
}
