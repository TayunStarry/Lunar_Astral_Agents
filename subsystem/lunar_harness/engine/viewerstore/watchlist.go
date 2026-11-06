package viewerstore

// watchlist.go 待看视频列表（watch_video）的入队、审核与查询。
//
// 并发策略：连接池固定单连接（见 variable.go 的 maxOpenConns），批量写操作再包一层事务；
// SQLite 的 busy_timeout=10s 负责跨进程兜底，因此生活链与观影流程同时读写的写操作天然串行。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EnqueueVideos 批量入队；同 BV 已存在时整条跳过（幂等），不覆盖已有审核结果与观看进度。
//
// 片段为空的候选也允许入队：审核通过后先落库，等下载阶段再用 ReplaceSegments 补写。
func EnqueueVideos(items []WatchCandidate) (EnqueueReport, error) {
	report := EnqueueReport{}
	candidates, err := normalizeCandidates(items)
	if err != nil {
		return report, err
	}
	if len(candidates) == 0 {
		return report, nil
	}
	db, err := openStore()
	if err != nil {
		return report, err
	}
	tx, err := db.Begin()
	if err != nil {
		return report, fmt.Errorf("开始入队事务失败: %w", err)
	}
	defer tx.Rollback()

	var seq int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(queue_seq), 0) FROM watch_video`).Scan(&seq); err != nil {
		return report, fmt.Errorf("读取入队顺序失败: %w", err)
	}
	now := time.Now().Unix()
	for _, item := range candidates {
		comments, err := json.Marshal(item.Comments)
		if err != nil {
			return report, fmt.Errorf("序列化评论摘录失败: %w", err)
		}
		nextSeq := seq + 1
		result, err := tx.Exec(`INSERT INTO watch_video
			(bv, title, intro, author_uid, author_name, comments, duration_sec,
			 status, queue_seq, queued_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(bv) DO NOTHING`,
			item.BV, item.Title, item.Intro, item.AuthorUID, item.AuthorName, string(comments),
			item.DurationSec, string(WatchStatusPending), nextSeq, now)
		if err != nil {
			return report, fmt.Errorf("写入待看视频失败: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return report, fmt.Errorf("读取入队结果失败: %w", err)
		}
		if affected == 0 {
			report.Skipped++
			continue
		}
		seq = nextSeq
		if err := insertSegments(tx, item.BV, item.Segments); err != nil {
			return report, err
		}
		report.Inserted++
		report.InsertedBVs = append(report.InsertedBVs, item.BV)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("提交入队事务失败: %w", err)
	}
	return report, nil
}

// SetVideoReview 更新审核结果；未通过时写入原因，其余状态清空原因。
func SetVideoReview(bv string, status WatchStatus, reason string) error {
	bv = strings.TrimSpace(bv)
	if bv == "" {
		return fmt.Errorf("待看视频缺少 BV 号")
	}
	if !validWatchStatus(status) {
		return fmt.Errorf("未知审核状态: %s", status)
	}
	if status != WatchStatusRejected {
		reason = ""
	}
	reason = cutRunes(strings.TrimSpace(reason), WatchRejectReasonMaxRunes)
	db, err := openStore()
	if err != nil {
		return err
	}
	result, err := db.Exec(`UPDATE watch_video
		SET status = ?, reject_reason = ?, reviewed_at = ? WHERE bv = ?`,
		string(status), reason, time.Now().Unix(), bv)
	if err != nil {
		return fmt.Errorf("更新审核结果失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取审核结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("待看视频不存在: %s", bv)
	}
	return nil
}

// LoadVideo 按 BV 号读取一条待看视频（不分审核状态、包含已归档）。
func LoadVideo(bv string) (WatchVideo, bool, error) {
	db, err := openStore()
	if err != nil {
		return WatchVideo{}, false, err
	}
	row := db.QueryRow(`SELECT `+watchVideoColumns+` FROM watch_video WHERE bv = ?`,
		strings.TrimSpace(bv))
	video, err := scanWatchVideo(row)
	if err == sql.ErrNoRows {
		return WatchVideo{}, false, nil
	}
	if err != nil {
		return WatchVideo{}, false, err
	}
	return video, true, nil
}

// LoadApprovedVideos 读取审核通过且未归档的待看列表（按入队顺序，含已看完的视频）。
func LoadApprovedVideos() ([]WatchVideo, error) {
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT `+watchVideoColumns+` FROM watch_video
		WHERE status = ? AND archived = 0 ORDER BY queue_seq ASC, bv ASC`,
		string(WatchStatusApproved))
	if err != nil {
		return nil, fmt.Errorf("读取待看列表失败: %w", err)
	}
	return scanWatchVideos(rows)
}

// WatchlistEmpty 待看列表是否为空（已通过、未归档、且还没看完的视频数为 0）。
//
// 供生活链判定「是否需要抓新视频」：已看完的视频不算待看，全部看完即视为空。
func WatchlistEmpty() (bool, error) {
	db, err := openStore()
	if err != nil {
		return false, err
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_video
		WHERE status = ? AND archived = 0 AND completed = 0`,
		string(WatchStatusApproved)).Scan(&count); err != nil {
		return false, fmt.Errorf("统计待看列表失败: %w", err)
	}
	return count == 0, nil
}

// ArchiveCompletedVideos 逻辑归档所有已看完的视频（保留历史行，完整视频统计不受影响）。
//
// 返回本次归档的条数。
func ArchiveCompletedVideos() (int, error) {
	db, err := openStore()
	if err != nil {
		return 0, err
	}
	result, err := db.Exec(`UPDATE watch_video SET archived = 1, archived_at = ?
		WHERE completed = 1 AND archived = 0`, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("归档已完成视频失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取归档结果失败: %w", err)
	}
	return int(affected), nil
}
