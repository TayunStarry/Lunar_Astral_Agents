package viewerstore

// watch_query.go 待看视频 / 片段的行扫描（查询列序与变量.go 的 watchVideoColumns 对齐）。

import (
	"database/sql"
	"encoding/json"
)

// rowScanner 兼容 *sql.Row 与 *sql.Rows。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanWatchVideo 把一行待看视频读成结构体。
func scanWatchVideo(scanner rowScanner) (WatchVideo, error) {
	var video WatchVideo
	var comments, topics, status string
	var completed, archived int
	if err := scanner.Scan(&video.BV, &video.Title, &video.Intro, &video.AuthorUID,
		&video.AuthorName, &comments, &video.DurationSec, &status, &video.RejectReason,
		&video.QueueSeq, &video.QueuedAt, &video.ReviewedAt, &completed,
		&video.CompletedAt, &archived, &video.ArchivedAt, &topics); err != nil {
		return WatchVideo{}, err
	}
	video.Status = WatchStatus(status)
	video.Completed = completed != 0
	video.Archived = archived != 0
	video.Comments = decodeWatchComments(comments)
	video.Topics = decodeWatchTopics(topics)
	return video, nil
}

// scanWatchVideos 读完整个结果集（内部负责关闭）。
func scanWatchVideos(rows *sql.Rows) ([]WatchVideo, error) {
	defer rows.Close()
	out := make([]WatchVideo, 0, 8)
	for rows.Next() {
		video, err := scanWatchVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, video)
	}
	return out, rows.Err()
}

// scanWatchSegment 把一行片段读成结构体。
func scanWatchSegment(scanner rowScanner) (WatchSegment, error) {
	var segment WatchSegment
	var downloaded, watched int
	if err := scanner.Scan(&segment.ID, &segment.BV, &segment.Index, &segment.StartSec,
		&segment.EndSec, &segment.Path, &downloaded, &watched, &segment.WatchedAt); err != nil {
		return WatchSegment{}, err
	}
	segment.Downloaded = downloaded != 0
	segment.Watched = watched != 0
	return segment, nil
}

// decodeWatchComments 还原评论摘录；脏数据按空列表处理，不让单条视频阻断整张列表。
func decodeWatchComments(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var comments []string
	if err := json.Unmarshal([]byte(raw), &comments); err != nil {
		return []string{}
	}
	return comments
}

// decodeWatchTopics 还原主题词；旧行为空串或脏数据一律按空列表处理。
func decodeWatchTopics(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var topics []string
	if err := json.Unmarshal([]byte(raw), &topics); err != nil {
		return []string{}
	}
	return topics
}
