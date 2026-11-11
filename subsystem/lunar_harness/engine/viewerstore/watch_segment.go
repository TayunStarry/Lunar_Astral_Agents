package viewerstore

// watch_segment.go 待看视频片段（watch_segment）的写入、续跑取片与完整视频计数。

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ReplaceSegments 整组覆盖一个视频的片段清单（重新切片段时使用），并重置完成状态。
//
// 删除与插入在同一事务内完成；视频不存在时直接报错，避免写出孤立片段。
func ReplaceSegments(bv string, segments []SegmentInput) error {
	bv = strings.TrimSpace(bv)
	if bv == "" {
		return fmt.Errorf("待看视频缺少 BV 号")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	var duration int
	err = db.QueryRow(`SELECT duration_sec FROM watch_video WHERE bv = ?`, bv).Scan(&duration)
	if err == sql.ErrNoRows {
		return fmt.Errorf("待看视频不存在: %s", bv)
	}
	if err != nil {
		return fmt.Errorf("读取视频时长失败: %w", err)
	}
	normalized, err := normalizeSegments(segments, duration)
	if err != nil {
		return fmt.Errorf("视频 %s 的片段不合法: %w", bv, err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始切片段事务失败: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM watch_segment WHERE bv = ?`, bv); err != nil {
		return fmt.Errorf("清理旧片段失败: %w", err)
	}
	if err := insertSegments(tx, bv, normalized); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE watch_video SET completed = 0, completed_at = 0 WHERE bv = ?`, bv); err != nil {
		return fmt.Errorf("重置完成状态失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交切片段事务失败: %w", err)
	}
	return nil
}

// LoadSegments 读取一个视频的全部片段（按序号升序）。
func LoadSegments(bv string) ([]WatchSegment, error) {
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, bv, seg_index, start_sec, end_sec, path,
		downloaded, watched, watched_at
		FROM watch_segment WHERE bv = ? ORDER BY seg_index ASC`, strings.TrimSpace(bv))
	if err != nil {
		return nil, fmt.Errorf("读取片段失败: %w", err)
	}
	defer rows.Close()
	out := make([]WatchSegment, 0, WatchSegmentMax)
	for rows.Next() {
		segment, err := scanWatchSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, segment)
	}
	return out, rows.Err()
}

// NextUnwatchedSegment 取下一个可播放片段：视频入队顺序 → 片段序号，只取已下载且未观看的。
//
// 没有可播放片段时返回 (零值, false, nil)；重启后同一查询直接续跑，不需要额外状态。
func NextUnwatchedSegment() (WatchSegment, bool, error) {
	db, err := openStore()
	if err != nil {
		return WatchSegment{}, false, err
	}
	row := db.QueryRow(`SELECT s.id, s.bv, s.seg_index, s.start_sec, s.end_sec, s.path,
		s.downloaded, s.watched, s.watched_at
		FROM watch_segment AS s
		JOIN watch_video AS v ON v.bv = s.bv
		WHERE s.watched = 0 AND s.downloaded = 1
			AND v.status = ? AND v.archived = 0
		ORDER BY v.queue_seq ASC, s.seg_index ASC, s.id ASC LIMIT 1`,
		string(WatchStatusApproved))
	segment, err := scanWatchSegment(row)
	if err == sql.ErrNoRows {
		return WatchSegment{}, false, nil
	}
	if err != nil {
		return WatchSegment{}, false, fmt.Errorf("读取下一个片段失败: %w", err)
	}
	return segment, true, nil
}

// MarkSegmentWatched 标记片段已看；该视频全部片段看完时同步置为完成。
//
// onCompleted 在「完成状态刚从 0 翻到 1」的同一事务内调用，供调用方把主题词并入偏好池；
// 传 nil 表示只做存储标记、不记录偏好。重复标记同一片段按幂等处理（保留首次观看时间），
// 并重新核对完成状态。
func MarkSegmentWatched(bv string, index int, at int64, onCompleted TopicBumpFunc) (SegmentWatchResult, error) {
	result := SegmentWatchResult{}
	bv = strings.TrimSpace(bv)
	if bv == "" {
		return result, fmt.Errorf("待看视频缺少 BV 号")
	}
	if at <= 0 {
		at = time.Now().Unix()
	}
	db, err := openStore()
	if err != nil {
		return result, err
	}
	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("开始标记事务失败: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM watch_segment WHERE bv = ? AND seg_index = ?`, bv, index).Scan(&exists)
	if err == sql.ErrNoRows {
		return result, fmt.Errorf("片段不存在: %s #%d", bv, index)
	}
	if err != nil {
		return result, fmt.Errorf("查询片段失败: %w", err)
	}
	if _, err := tx.Exec(`UPDATE watch_segment SET watched = 1, watched_at = ?
		WHERE bv = ? AND seg_index = ? AND watched = 0`, at, bv, index); err != nil {
		return result, fmt.Errorf("标记片段已看失败: %w", err)
	}

	var total, unwatched int
	if err := tx.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN watched = 0 THEN 1 ELSE 0 END), 0)
		FROM watch_segment WHERE bv = ?`, bv).Scan(&total, &unwatched); err != nil {
		return result, fmt.Errorf("统计片段进度失败: %w", err)
	}
	if total == 0 {
		return result, fmt.Errorf("视频 %s 没有片段，无法标记观看", bv)
	}
	result.VideoCompleted = unwatched == 0
	if result.VideoCompleted {
		// 只有完成状态真正从 0 翻到 1 的那一次才给主题词加权，
		// 重复标记同一片段不会把同一支视频反复计入偏好。
		updated, err := tx.Exec(`UPDATE watch_video
			SET completed = 1,
				completed_at = CASE WHEN completed_at = 0 THEN ? ELSE completed_at END
			WHERE bv = ? AND completed = 0`, at, bv)
		if err != nil {
			return result, fmt.Errorf("标记视频完成失败: %w", err)
		}
		changed, err := updated.RowsAffected()
		if err != nil {
			return result, fmt.Errorf("读取完成状态结果失败: %w", err)
		}
		if changed > 0 && onCompleted != nil {
			var topics string
			if err := tx.QueryRow(`SELECT topics FROM watch_video WHERE bv = ?`, bv).Scan(&topics); err != nil {
				return result, fmt.Errorf("读取视频主题词失败: %w", err)
			}
			// 偏好策略由调用方注入，但必须复用本事务，保证完成状态与偏好加权原子落盘。
			if err := onCompleted(topicPoolTx{tx: tx}, decodeWatchTopics(topics), at); err != nil {
				return result, fmt.Errorf("更新主题词偏好失败: %w", err)
			}
		}
	}
	count, err := countCompletedVideos(tx)
	if err != nil {
		return result, err
	}
	result.CompletedVideos = count
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("提交标记事务失败: %w", err)
	}
	return result, nil
}

// ClearSegmentPath 清空片段的本地文件路径（文件删除后调用），保留观看与下载标记。
func ClearSegmentPath(bv string, index int) error {
	bv = strings.TrimSpace(bv)
	if bv == "" {
		return fmt.Errorf("待看视频缺少 BV 号")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE watch_segment SET path = '' WHERE bv = ? AND seg_index = ?`, bv, index); err != nil {
		return fmt.Errorf("清空片段路径失败: %w", err)
	}
	return nil
}

// CountCompletedVideos 统计完整视频数（全部片段都看完的视频，含已归档的历史行）。
func CountCompletedVideos() (int, error) {
	db, err := openStore()
	if err != nil {
		return 0, err
	}
	return countCompletedVideos(db)
}

// insertSegments 按给定顺序写入片段，序号即切片后的位置（已成片段清单的唯一来源）。
func insertSegments(tx *sql.Tx, bv string, segments []SegmentInput) error {
	for index, segment := range segments {
		if _, err := tx.Exec(`INSERT INTO watch_segment
			(bv, seg_index, start_sec, end_sec, path, downloaded)
			VALUES (?, ?, ?, ?, ?, ?)`,
			bv, index, segment.StartSec, segment.EndSec, segment.Path,
			boolToInt(segment.Downloaded)); err != nil {
			return fmt.Errorf("写入片段失败: %w", err)
		}
	}
	return nil
}

// countCompletedVideos 统计完整视频数（事务内共用）。
func countCompletedVideos(q interface {
	QueryRow(query string, args ...any) *sql.Row
}) (int, error) {
	var count int
	if err := q.QueryRow(`SELECT COUNT(*) FROM watch_video WHERE completed = 1`).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计完整视频数失败: %w", err)
	}
	return count, nil
}

// boolToInt SQLite 没有布尔类型，统一按 0/1 存储。
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
