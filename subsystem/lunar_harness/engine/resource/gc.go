package resource

// gc.go 配额回收与目录统计的查询原语（GC 判据 / 引用计数 / 运维视图）。

// totalActiveBytes 统计 state=active 的资源总体积（GC 配额判据）
func (d *store) totalActiveBytes() (int64, error) {
	var total int64
	err := d.db.QueryRow(`SELECT COALESCE(SUM(size_bytes),0) FROM resources WHERE state = ?`, StateActive).Scan(&total)
	return total, err
}

// collectCandidates 取指定保留等级的回收候选：state=active，按 used_at 升序（最久未用优先）
func (d *store) collectCandidates(retain string, limit int) ([]Resource, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.db.Query(`SELECT `+archiveResourceColumns+
		` FROM resources WHERE state = ? AND retain = ? ORDER BY used_at ASC, id ASC LIMIT ?`,
		StateActive, retain, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// blobReferencedByOthers 判断同一实体是否仍被其它目录行引用（共享去重保护）
func (d *store) blobReferencedByOthers(blobPath string, selfID int64) (bool, error) {
	if blobPath == "" {
		return false, nil
	}
	var count int64
	err := d.db.QueryRow(`SELECT COUNT(*) FROM resources WHERE blob_path = ? AND id <> ?`,
		blobPath, selfID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// blobReferencedByUnowned 判断同一实体是否被「非归档所有」的目录行引用
//
// 内容寻址下同内容共用实体：若任一行标记该实体归前端/引擎所有（blob_owned=0），
// 那么它就是用户可见文件，回收器一律不得删除 —— 归属按最保守的一侧处理。
func (d *store) blobReferencedByUnowned(blobPath string, selfID int64) (bool, error) {
	if blobPath == "" {
		return false, nil
	}
	var count int64
	err := d.db.QueryRow(`SELECT COUNT(*) FROM resources WHERE blob_path = ? AND id <> ? AND blob_owned = 0`,
		blobPath, selfID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Stats 目录统计
func (d *store) Stats() (StatsReport, error) {
	stats := StatsReport{ByKind: map[string]int64{}, ByState: map[string]int64{}, ByRetain: map[string]int64{}}
	if err := d.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size_bytes),0) FROM resources`).
		Scan(&stats.Total, &stats.Bytes); err != nil {
		return stats, err
	}
	for _, group := range []struct {
		column string
		target map[string]int64
	}{
		{"kind", stats.ByKind},
		{"state", stats.ByState},
		{"retain", stats.ByRetain},
	} {
		rows, err := d.db.Query(`SELECT ` + group.column + `, COUNT(*) FROM resources GROUP BY ` + group.column)
		if err != nil {
			continue
		}
		for rows.Next() {
			var key string
			var count int64
			if err := rows.Scan(&key, &count); err == nil {
				group.target[key] = count
			}
		}
		rows.Close()
	}
	return stats, nil
}
