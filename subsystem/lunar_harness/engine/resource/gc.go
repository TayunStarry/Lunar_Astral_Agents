package resource

// gc.go 配额回收与目录统计的查询原语（GC 判据 / 运维视图）

// totalActiveBytes 统计 state=active 且已绑定句柄的资源总体积
func (d *store) totalActiveBytes() (int64, error) {
	var total int64
	err := d.db.QueryRow(`SELECT COALESCE(SUM(size_bytes),0) FROM url_master
		WHERE state = ? AND bound_handle <> ''`, StateActive).Scan(&total)
	return total, err
}

// collectCandidates 取指定保留等级的回收候选：active，按 used_at 升序（最久未用优先）
func (d *store) collectCandidates(retain string, limit int) ([]Resource, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.db.Query(`SELECT `+urlMasterColumns+` FROM url_master
		WHERE state = ? AND retain = ? AND bound_handle <> '' ORDER BY used_at ASC, id ASC LIMIT ?`,
		StateActive, retain, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		res, scanErr := scanURLRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// Stats 目录统计（只统计已绑定句柄的资源）
func (d *store) Stats() (StatsReport, error) {
	stats := StatsReport{ByKind: map[string]int64{}, ByState: map[string]int64{}, ByRetain: map[string]int64{}}
	if err := d.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size_bytes),0) FROM url_master
		WHERE bound_handle <> ''`).Scan(&stats.Total, &stats.Bytes); err != nil {
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
		rows, err := d.db.Query(`SELECT ` + group.column + `, COUNT(*) FROM url_master
			WHERE bound_handle <> '' GROUP BY ` + group.column)
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
