package viewerstore

// inbox_seen.go 消息中心去重键（inbox_seen）的行级读写。
//
// 一条互动会同时命中「消息 ID」与「来源评论」两个去重键，键本身就是业务唯一键，
// 因此直接作为主键；既不压缩成一行，也不改写历史前缀。
// TTL 与条数上限属于业务口径，由调用方把 cutoff / max 传进来，本文件只负责事务内落库。

import (
	"fmt"
)

// LoadInboxSeenKeys 读取全部去重键及其最后处理时间。
//
// 表有调用方维护的条数上限，整表加载的成本可以忽略；读取时不做 TTL 过滤，
// 与旧实现「文件里有什么就返回什么」保持一致。
func LoadInboxSeenKeys() (map[string]int64, error) {
	db, err := openStore()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT key, seen_at FROM inbox_seen`)
	if err != nil {
		return nil, fmt.Errorf("读取消息去重记录失败: %w", err)
	}
	out := make(map[string]int64)
	for rows.Next() {
		var row InboxSeenRow
		if err := rows.Scan(&row.Key, &row.SeenAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("解析消息去重记录失败: %w", err)
		}
		out[row.Key] = row.SeenAt
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("读取消息去重记录失败: %w", err)
	}
	rows.Close()
	return out, nil
}

// MarkInboxSeenKeys 记录已处理的消息键，并在同一事务内完成过期清理与条数截断。
//
// cutoff 之前（不含）的记录会被删除；max > 0 时只保留 seen_at 最新的 max 条。
// 同一键重复写入会刷新时间戳，与旧实现的 map 覆盖语义一致。
func MarkInboxSeenKeys(keys []string, now, cutoff int64, max int) error {
	unique := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, key)
	}
	if len(unique) == 0 {
		return nil
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始去重记录事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO inbox_seen(key, seen_at) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET seen_at = excluded.seen_at`)
	if err != nil {
		return fmt.Errorf("准备去重记录写入失败: %w", err)
	}
	for _, key := range unique {
		if _, err := stmt.Exec(key, now); err != nil {
			stmt.Close()
			return fmt.Errorf("写入去重记录失败: %w", err)
		}
	}
	stmt.Close()

	if _, err := tx.Exec(`DELETE FROM inbox_seen WHERE seen_at < ?`, cutoff); err != nil {
		return fmt.Errorf("清理过期去重记录失败: %w", err)
	}
	if max > 0 {
		if _, err := tx.Exec(`DELETE FROM inbox_seen WHERE key NOT IN (
			SELECT key FROM inbox_seen ORDER BY seen_at DESC, key ASC LIMIT ?)`, max); err != nil {
			return fmt.Errorf("截断去重记录失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交去重记录事务失败: %w", err)
	}
	return nil
}
