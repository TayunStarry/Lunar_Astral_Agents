package viewerstore

// seen_video.go 跨轮已看视频（seen_video）的行级读写。
//
// 语义是「一个 BV 一条，记录最近一次观看时间」：同一 BV 重复写入只刷新时间，
// 不会产生第二行，因此主键是 bv 而不是 (bv, watched_at)。

import (
	"fmt"
)

// LoadSeenVideosSince 返回 watched_at >= since 的 BV 集合（保留期过滤由调用方给出）。
func LoadSeenVideosSince(since int64) (map[string]bool, error) {
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT bv FROM seen_video WHERE watched_at >= ?`, since)
	if err != nil {
		return nil, fmt.Errorf("读取已看记录失败: %w", err)
	}
	out := make(map[string]bool)
	for rows.Next() {
		var bv string
		if err := rows.Scan(&bv); err != nil {
			rows.Close()
			return nil, fmt.Errorf("解析已看记录失败: %w", err)
		}
		out[bv] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("读取已看记录失败: %w", err)
	}
	rows.Close()
	return out, nil
}

// RecordSeenVideos 批量登记已看视频；同一 BV 取较新的时间。
//
// 供历史迁移与测试构造前置状态使用，生产阅读流程目前没有写入点。
func RecordSeenVideos(records map[string]int64) error {
	if len(records) == 0 {
		return nil
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始已看记录事务失败: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO seen_video(bv, watched_at) VALUES(?, ?)
		ON CONFLICT(bv) DO UPDATE SET watched_at = MAX(seen_video.watched_at, excluded.watched_at)`)
	if err != nil {
		return fmt.Errorf("准备已看记录写入失败: %w", err)
	}
	for bv, at := range records {
		if bv == "" {
			continue
		}
		if _, err := stmt.Exec(bv, at); err != nil {
			stmt.Close()
			return fmt.Errorf("写入已看记录失败: %w", err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交已看记录事务失败: %w", err)
	}
	return nil
}
